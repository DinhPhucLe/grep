// One-shot local importer: stream mock_dev_knowledge_base.csv into MongoDB
// knowledge_documents (fail-soft, ~1500 cap). scripts/oneoff/ is gitignored.
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	collectionName  = "knowledge_documents"
	vectorIndexName = "knowledge_content_voyage"
	embeddingModel  = "voyage-code-4"
	embeddingDims   = 1024
	defaultLimit    = 1500
	maxContentRunes = 4000
	maxTopics       = 16
	defaultOrgID    = "660100000000000004000000" // NovaPay demo cast
	defaultCSVName  = "mock_dev_knowledge_base.csv"
)

func main() {
	log.SetFlags(0)
	loadDotEnvs()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func loadDotEnvs() {
	candidates := []string{".env", "server/.env", "../../../server/.env"}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 8; i++ {
			candidates = append(candidates, filepath.Join(dir, "server", ".env"), filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	seen := map[string]bool{}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if err := godotenv.Load(p); err == nil {
			log.Printf("loaded env file %s", abs)
		}
	}
}

func run() error {
	limit := defaultLimit
	if raw := os.Getenv("SO_IMPORT_LIMIT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return fmt.Errorf("SO_IMPORT_LIMIT must be a positive int")
		}
		limit = n
	}
	orgID := envOr("ORGANIZATION_ID", defaultOrgID)
	csvPath, err := resolveCSVPath()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	mongoDB, err := openMongo(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = mongoDB.Client().Disconnect(context.Background()) }()

	if err := ensureKnowledgeSchema(ctx, mongoDB); err != nil {
		return err
	}

	f, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.ReuseRecord = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read csv header: %w", err)
	}
	col := map[string]int{}
	for i, name := range header {
		col[strings.ToLower(strings.TrimSpace(name))] = i
	}
	for _, need := range []string{"id", "title"} {
		if _, ok := col[need]; !ok {
			return fmt.Errorf("csv missing required column %q (have %v)", need, header)
		}
	}
	log.Printf("source=%s rows_cap=%d org=%s", csvPath, limit, orgID)

	coll := mongoDB.Collection(collectionName)
	var scanned, inserted, skipped, failed int

	for {
		if inserted >= limit {
			break
		}
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		scanned++
		if err != nil {
			failed++
			printStatus(scanned, "error", "?", "", err.Error())
			continue
		}

		row := rowMap(col, record)
		docID := field(row, "id")
		title := field(row, "title")
		doc, skipReason, err := mapCSVRow(row, orgID)
		if err != nil {
			failed++
			printStatus(scanned, "error", docID, title, err.Error())
			continue
		}
		if skipReason != "" {
			skipped++
			printStatus(scanned, "skip", docID, title, skipReason)
			continue
		}

		upsertCtx, upsertCancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = coll.UpdateOne(upsertCtx,
			bson.M{"_id": doc.ID},
			bson.M{"$setOnInsert": doc},
			options.UpdateOne().SetUpsert(true),
		)
		upsertCancel()
		if err != nil {
			failed++
			printStatus(scanned, "error", docID, title, "insert: "+err.Error())
			continue
		}
		inserted++
		printStatus(scanned, "ok", docID, title, fmt.Sprintf("mongo_id=%s", doc.ID.Hex()))
	}

	log.Printf("summary scanned=%d inserted=%d skipped=%d errors=%d", scanned, inserted, skipped, failed)

	created, err := ensureVectorIndex(ctx, mongoDB)
	if err != nil {
		log.Printf("vector index warning: %v", err)
		log.Printf("create Atlas index %q manually (voyage-code-4 autoEmbed on content) if needed", vectorIndexName)
	} else if created {
		log.Printf("created search index %s (%s); wait until READY before $vectorSearch", vectorIndexName, embeddingModel)
	} else {
		log.Printf("search index %s already present", vectorIndexName)
	}
	return nil
}

func resolveCSVPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("MOCK_CSV_PATH")); p != "" {
		return p, nil
	}
	candidates := []string{
		defaultCSVName,
		filepath.Join("scripts", "oneoff", "so_cloud_to_mongo", defaultCSVName),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, defaultCSVName))
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs, nil
		}
	}
	return "", fmt.Errorf("mock csv not found; set MOCK_CSV_PATH or place %s next to the binary", defaultCSVName)
}

func printStatus(n int, status, id, title, detail string) {
	preview := strings.ReplaceAll(title, "\n", " ")
	if utf8.RuneCountInString(preview) > 60 {
		preview = string([]rune(preview)[:60]) + "…"
	}
	if id == "" {
		id = "?"
	}
	if detail == "" {
		log.Printf("[%d] %s id=%s title=%q", n, status, id, preview)
		return
	}
	log.Printf("[%d] %s id=%s title=%q detail=%s", n, status, id, preview, detail)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func openMongo(ctx context.Context) (*mongo.Database, error) {
	uri := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	name := strings.TrimSpace(os.Getenv("MONGODB_DATABASE"))
	if uri == "" || name == "" {
		return nil, fmt.Errorf("missing env: MONGODB_URI and MONGODB_DATABASE")
	}
	if strings.Contains(uri, "<db_username>") {
		u := os.Getenv("DB_USERNAME")
		if u == "" {
			return nil, fmt.Errorf("DB_USERNAME required for MONGODB_URI placeholder")
		}
		uri = strings.ReplaceAll(uri, "<db_username>", url.User(u).String())
	}
	if strings.Contains(uri, "<db_password>") {
		p := os.Getenv("DB_PASSWORD")
		if p == "" {
			return nil, fmt.Errorf("DB_PASSWORD required for MONGODB_URI placeholder")
		}
		escaped := strings.TrimPrefix(url.UserPassword("", p).String(), ":")
		uri = strings.ReplaceAll(uri, "<db_password>", escaped)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client.Database(name), nil
}

func rowMap(col map[string]int, record []string) map[string]string {
	out := make(map[string]string, len(col))
	for name, i := range col {
		if i < len(record) {
			out[name] = record[i]
		}
	}
	return out
}

func field(row map[string]string, key string) string {
	return strings.TrimSpace(row[key])
}

type knowledgeDoc struct {
	ID             bson.ObjectID     `bson:"_id"`
	Content        string            `bson:"content"`
	Topics         []string          `bson:"topics"`
	Properties     map[string]string `bson:"properties"`
	Authors        []knowledgeAuthor `bson:"authors"`
	CreatedAt      time.Time         `bson:"created_at"`
	UpdatedAt      time.Time         `bson:"updated_at"`
	OrganizationID string            `bson:"organization_id"`
}

type knowledgeAuthor struct {
	UserID string `bson:"user_id"`
	Name   string `bson:"name,omitempty"`
}

func mapCSVRow(row map[string]string, orgID string) (knowledgeDoc, string, error) {
	id := field(row, "id")
	if id == "" {
		return knowledgeDoc{}, "missing id", nil
	}
	title := field(row, "title")
	body := field(row, "body")
	answer := field(row, "accepted_answer")
	content := field(row, "content")
	if content == "" {
		parts := []string{}
		if title != "" {
			parts = append(parts, title)
		}
		if body != "" {
			parts = append(parts, body)
		}
		if answer != "" {
			parts = append(parts, "Accepted answer:\n"+answer)
		}
		content = strings.Join(parts, "\n\n")
	} else if answer != "" && !strings.Contains(content, answer[:min(40, len(answer))]) {
		content = content + "\n\nAccepted answer:\n" + answer
	}
	content = truncateRunes(strings.TrimSpace(content), maxContentRunes)
	if content == "" {
		return knowledgeDoc{}, "empty content", nil
	}

	topics := parseTags(field(row, "tags"))
	for _, extra := range []string{field(row, "primary_technology"), field(row, "category")} {
		extra = strings.ToLower(strings.TrimSpace(extra))
		if extra == "" {
			continue
		}
		topics = appendUnique(topics, extra)
	}
	if len(topics) == 0 {
		topics = []string{"engineering"}
	}
	if len(topics) > maxTopics {
		topics = topics[:maxTopics]
	}

	created := time.Now().UTC()
	if raw := field(row, "created_date"); raw != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, raw); err == nil {
				created = t.UTC()
				break
			}
		}
	}

	authorName := field(row, "author")
	authorID := "mock-author"
	if authorName != "" {
		authorID = "mock-" + slug(authorName)
	}

	props := map[string]string{
		"source":   "mock_dev_knowledge_base",
		"mock_id":  id,
		"doc_type": field(row, "doc_type"),
	}
	for _, k := range []string{"primary_technology", "category", "score", "view_count", "answer_count", "is_answered", "has_code"} {
		if v := field(row, k); v != "" {
			props[k] = v
		}
	}

	return knowledgeDoc{
		ID:             idFromMock(id),
		Content:        content,
		Topics:         topics,
		Properties:     props,
		Authors:        []knowledgeAuthor{{UserID: authorID, Name: authorName}},
		CreatedAt:      created,
		UpdatedAt:      created,
		OrganizationID: orgID,
	}, "", nil
}

func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '<' || r == '>'
	})
	var out []string
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" || len(t) > 64 {
			continue
		}
		out = appendUnique(out, t)
		if len(out) >= maxTopics {
			break
		}
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func idFromMock(mockID string) bson.ObjectID {
	h := fnv.New64a()
	_, _ = h.Write([]byte("mock-dev-kb:" + mockID))
	sum := h.Sum64()
	var id bson.ObjectID
	id[0] = 0x67
	id[1] = 0x4d // M
	id[2] = 0x4b // K
	for i := 0; i < 9; i++ {
		id[3+i] = byte(sum >> (8 * (8 - i)))
	}
	return id
}

func ensureKnowledgeSchema(ctx context.Context, database *mongo.Database) error {
	names, err := database.ListCollectionNames(ctx, bson.D{{Key: "name", Value: collectionName}})
	if err != nil {
		return err
	}
	if len(names) == 0 {
		validator := bson.M{
			"$jsonSchema": bson.M{
				"bsonType":             "object",
				"required":             []string{"_id", "content", "topics", "properties", "authors", "created_at", "updated_at", "organization_id"},
				"additionalProperties": false,
				"properties": bson.M{
					"_id":     bson.M{"bsonType": "objectId"},
					"content": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 65536},
					"topics": bson.M{
						"bsonType": "array", "minItems": 0, "maxItems": 32,
						"items": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
					},
					"properties": bson.M{"bsonType": "object"},
					"authors": bson.M{
						"bsonType": "array", "minItems": 1, "maxItems": 8,
						"items": bson.M{
							"bsonType": "object", "required": []string{"user_id"}, "additionalProperties": false,
							"properties": bson.M{
								"user_id": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
								"name":    bson.M{"bsonType": "string", "maxLength": 256},
							},
						},
					},
					"created_at":      bson.M{"bsonType": "date"},
					"updated_at":      bson.M{"bsonType": "date"},
					"organization_id": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
				},
			},
		}
		if err := database.CreateCollection(ctx, collectionName, options.CreateCollection().
			SetValidator(validator).SetValidationLevel("strict").SetValidationAction("error")); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
	}
	_, err = database.Collection(collectionName).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("knowledge_documents_by_org_created")},
		{Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "topics", Value: 1}}, Options: options.Index().SetName("knowledge_documents_by_org_topics")},
		{Keys: bson.D{{Key: "properties.mock_id", Value: 1}}, Options: options.Index().SetName("knowledge_documents_by_mock_id")},
	})
	return err
}

func ensureVectorIndex(ctx context.Context, database *mongo.Database) (bool, error) {
	coll := database.Collection(collectionName)
	view := coll.SearchIndexes()
	cursor, err := view.List(ctx, options.SearchIndexes().SetName(vectorIndexName))
	if err != nil {
		return false, err
	}
	defer cursor.Close(ctx)
	if cursor.Next(ctx) {
		return false, nil
	}
	if err := cursor.Err(); err != nil {
		return false, err
	}
	def := bson.M{
		"fields": bson.A{
			bson.M{
				"type": "autoEmbed", "modality": "text", "path": "content",
				"model": embeddingModel, "numDimensions": embeddingDims, "similarity": "cosine",
			},
			bson.M{"type": "filter", "path": "organization_id"},
			bson.M{"type": "filter", "path": "topics"},
		},
	}
	_, err = view.CreateOne(ctx, mongo.SearchIndexModel{
		Definition: def,
		Options:    options.SearchIndexes().SetName(vectorIndexName).SetType("vectorSearch"),
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
