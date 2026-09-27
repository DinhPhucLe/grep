package seed

import (
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	kindUser    byte = 1
	kindProject byte = 2
	kindSession byte = 3
	kindOrg     byte = 4
	kindMember  byte = 5
	kindKnowledgeDoc byte = 7
	kindKnowledgeConnect byte = 8

	orgNovaPay     byte = 1
	orgAtlasHealth byte = 2

	employeesPerOrg = 25
	sharedReposPerOrg = 3

	// Featured local user indices within their org (1-based).
	featuredAlexLocal   byte = 1
	featuredJordanLocal byte = 2
	featuredSamLocal    byte = 1
)

// DemoCast holds stable ObjectIds for hackathon demo URLs.
type DemoCast struct {
	NovaPayOrgID     bson.ObjectID
	AtlasHealthOrgID bson.ObjectID
	AlexRiveraID     bson.ObjectID
	JordanKimID      bson.ObjectID
	SamOkonkwoID     bson.ObjectID
}

// DemoCastIDs returns the fixed org and featured-employee IDs used by the seed.
func DemoCastIDs() DemoCast {
	return DemoCast{
		NovaPayOrgID:     seedID(kindOrg, orgNovaPay, 0, 0, 0),
		AtlasHealthOrgID: seedID(kindOrg, orgAtlasHealth, 0, 0, 0),
		AlexRiveraID:     seedID(kindUser, orgNovaPay, featuredAlexLocal, 0, 0),
		JordanKimID:      seedID(kindUser, orgNovaPay, featuredJordanLocal, 0, 0),
		SamOkonkwoID:     seedID(kindUser, orgAtlasHealth, featuredSamLocal, 0, 0),
	}
}

type fileTarget struct {
	repoName string
	repoOrg  string
	module   string
	filePath string
	weight   int // relative weight for hotspot selection
}

type orgSpec struct {
	orgByte  byte
	name     string
	repoOrg  string
	repos    []string
	files    []fileTarget
	names    []string
	improving bool // NovaPay: rising correct rate; Atlas: sticky failures
}

// GENERATE TWO ORGS WITH SHARED CODEBASES, 25 EMPLOYEES EACH, AND PATTERNED PRACTICE EVENTS
func demoRecords() []seedRecord {
	var records []seedRecord
	base := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)

	nova := orgSpec{
		orgByte:   orgNovaPay,
		name:      "NovaPay",
		repoOrg:   "novapay",
		repos:     []string{"novapay-api", "novapay-web", "novapay-cli"},
		improving: true,
		names: []string{
			"Alex Rivera", "Jordan Kim", "Casey Nguyen", "Riley Patel", "Morgan Ellis",
			"Avery Brooks", "Quinn Morales", "Reese Cooper", "Harper Diaz", "Rowan Blake",
			"Cameron Soto", "Drew Vance", "Jamie Ortiz", "Parker Chen", "Taylor Brooks",
			"Skyler Reed", "Finley Hayes", "Emerson Cruz", "Dakota Price", "Phoenix Lane",
			"Sage Romero", "Blair Kent", "Elliot Nash", "Remy Cole", "Logan Pierce",
		},
		files: []fileTarget{
			{repoName: "novapay-api", module: "payments", filePath: "payments/checkout.go", weight: 5},
			{repoName: "novapay-api", module: "payments", filePath: "payments/refund.go", weight: 4},
			{repoName: "novapay-api", module: "payments", filePath: "payments/ledger.go", weight: 3},
			{repoName: "novapay-api", module: "auth", filePath: "auth/session.go", weight: 4},
			{repoName: "novapay-api", module: "auth", filePath: "auth/oauth.go", weight: 3},
			{repoName: "novapay-web", module: "web", filePath: "src/checkout.tsx", weight: 2},
			{repoName: "novapay-web", module: "web", filePath: "src/wallet.tsx", weight: 2},
			{repoName: "novapay-cli", module: "cli", filePath: "cmd/pay/main.go", weight: 1},
			{repoName: "novapay-cli", module: "cli", filePath: "internal/transfer.go", weight: 1},
		},
	}
	atlas := orgSpec{
		orgByte:   orgAtlasHealth,
		name:      "AtlasHealth",
		repoOrg:   "atlashealth",
		repos:     []string{"atlas-core", "atlas-billing", "atlas-portal"},
		improving: false,
		names: []string{
			"Sam Okonkwo", "Nina Vargas", "Chris Adeyemi", "Priya Shah", "Marcus Bell",
			"Elena Rossi", "Omar Farouk", "Lila Cho", "Ben Travers", "Sofia Mendes",
			"Kai Nakamura", "Amelia Frost", "Noah Berg", "Isla Quinn", "Leo Hartmann",
			"Maya Okada", "Ethan Brooks", "Zoe Keller", "Luke Anders", "Aria Singh",
			"Owen Clarke", "Nora Jimenez", "Felix Braun", "Chloe Park", "Hugo Martins",
		},
		files: []fileTarget{
			{repoName: "atlas-core", module: "legacy", filePath: "legacy/claims.go", weight: 5},
			{repoName: "atlas-core", module: "legacy", filePath: "legacy/eligibility.go", weight: 4},
			{repoName: "atlas-core", module: "legacy", filePath: "legacy/member.go", weight: 3},
			{repoName: "atlas-billing", module: "billing", filePath: "billing/invoice.go", weight: 5},
			{repoName: "atlas-billing", module: "billing", filePath: "billing/adjust.go", weight: 4},
			{repoName: "atlas-billing", module: "billing", filePath: "billing/payout.go", weight: 3},
			{repoName: "atlas-portal", module: "portal", filePath: "ui/dashboard.tsx", weight: 2},
			{repoName: "atlas-portal", module: "portal", filePath: "ui/claims.tsx", weight: 2},
			{repoName: "atlas-core", module: "api", filePath: "api/handler.go", weight: 1},
		},
	}

	for _, org := range []orgSpec{nova, atlas} {
		records = append(records, buildOrgRecords(org, base)...)
	}
	return records
}

func buildOrgRecords(org orgSpec, base time.Time) []seedRecord {
	var records []seedRecord
	orgID := seedID(kindOrg, org.orgByte, 0, 0, 0)
	created := base.Add(time.Duration(org.orgByte) * 24 * time.Hour)
	records = append(records, seedRecord{"organizations", bson.M{
		"_id": orgID, "name": org.name, "created_at": created,
	}})

	adminID := seedID(kindUser, org.orgByte, 1, 0, 0)
	userIDs := make([]bson.ObjectID, employeesPerOrg+1)
	for local := byte(1); local <= employeesPerOrg; local++ {
		userID := seedID(kindUser, org.orgByte, local, 0, 0)
		userIDs[local] = userID
		name := org.names[local-1]
		mailLocal := strings.ToLower(strings.ReplaceAll(name, " ", "."))
		userCreated := created.Add(time.Duration(local) * time.Hour)
		records = append(records, seedRecord{"users", bson.M{
			"_id": userID, "name": name, "mail": mailLocal + "@" + org.repoOrg + ".example",
			"created_at": userCreated,
		}})
		role := "member"
		if local == 1 {
			role = "admin"
		}
		records = append(records, seedRecord{"organization_members", bson.M{
			"_id":             seedID(kindMember, org.orgByte, local, 0, 0),
			"organization_id": orgID, "user_id": userID,
			"role": role, "joined_at": userCreated,
		}})
	}

	projectIDs := make([]bson.ObjectID, sharedReposPerOrg)
	sessionIDs := make([]bson.ObjectID, sharedReposPerOrg)
	repoToProject := map[string]int{}
	for i, repo := range org.repos {
		project := byte(i + 1)
		projectID := seedID(kindProject, org.orgByte, 1, project, 0)
		sessionID := seedID(kindSession, org.orgByte, 1, project, 1)
		projectIDs[i] = projectID
		sessionIDs[i] = sessionID
		repoToProject[repo] = i
		started := created.Add(time.Duration(i+1) * time.Hour)
		records = append(records, seedRecord{"projects", bson.M{
			"_id": projectID, "user_id": adminID, "name": repo,
			"root_path": fmt.Sprintf("/demo/%s/%s", strings.ToLower(org.name), repo),
			"repo_url":  fmt.Sprintf("https://example.com/%s/%s.git", org.repoOrg, repo),
		}})
		records = append(records, seedRecord{"sessions", bson.M{
			"_id": sessionID, "project_id": projectID,
			"agent_model": "demo-model", "started_at": started,
			"ended_at": started.Add(45 * time.Minute),
			"branch":   "main", "status": "complete", "shell": "zsh",
		}})
	}

	for local := byte(1); local <= employeesPerOrg; local++ {
		records = append(records, practiceEventsForUser(
			org, orgID, local, userIDs[local], projectIDs, sessionIDs, repoToProject,
		)...)
	}
	if org.orgByte == orgNovaPay || org.orgByte == orgAtlasHealth {
		records = append(records, knowledgeRecordsForOrg(org, orgID, userIDs, created)...)
	}
	return records
}

func knowledgeRecordsForOrg(org orgSpec, orgID bson.ObjectID, userIDs []bson.ObjectID, base time.Time) []seedRecord {
	// Featured trio per org: locals 1, 2, 3 — every demo-cast employee must have in+out degrees.
	a := userIDs[1]
	b := userIDs[2]
	c := userIDs[3]
	nameByID := map[string]string{
		a.Hex(): org.names[0],
		b.Hex(): org.names[1],
		c.Hex(): org.names[2],
	}

	type card struct {
		seq     byte
		author  bson.ObjectID
		topics  []string
		content string
	}
	var cards []card
	var repo string
	if org.orgByte == orgNovaPay {
		repo = "payments-api"
		cards = []card{
			{1, b, []string{"payments", "retries"}, "Prefer exponential backoff with jitter on payment gateway 429s."},
			{2, a, []string{"payments", "ledger"}, "Ledger postings must be idempotent on (org, invoice_id)."},
			{3, b, []string{"auth", "webhooks"}, "Webhook signatures verify against the rotating signing secret."},
			{4, a, []string{"retries", "observability"}, "Emit retry_attempt metric before the sleep, not after."},
			{5, c, []string{"ledger", "payments"}, "Never mutate posted ledger rows; create reversing entries."},
			{6, b, []string{"webhooks", "payments"}, "Replay webhooks through the dead-letter topic, not the live consumer."},
			{7, c, []string{"auth", "retries"}, "Cache JWKS for at most five minutes; force refresh on kid miss."},
			{8, a, []string{"observability", "webhooks"}, "Trace webhook delivery with the same request id as the producer."},
		}
	} else {
		repo = "atlas-core"
		// Same author pattern as NovaPay (b,a,b,a,c,b,c,a) so one edge table works for both orgs.
		cards = []card{
			{1, b, []string{"claims", "eligibility"}, "Eligibility checks must short-circuit on terminated members."},
			{2, a, []string{"claims", "billing"}, "Claim adjustments require a dual-control approval on amounts over 500."},
			{3, b, []string{"billing", "payouts"}, "Payout batches are idempotent on (org, batch_id)."},
			{4, a, []string{"portal", "claims"}, "Portal claim lists page by updated_at, never by insertion order."},
			{5, c, []string{"eligibility", "api"}, "Legacy eligibility SOAP faults map to typed domain errors."},
			{6, b, []string{"billing", "claims"}, "Do not reopen closed claim years from the portal write path."},
			{7, c, []string{"payouts", "billing"}, "Payout retries use the same ledger hold until settlement clears."},
			{8, a, []string{"portal", "api"}, "Portal BFF caches member cards for 30s keyed by member_id."},
		}
	}

	var records []seedRecord
	docIDs := make([]bson.ObjectID, len(cards))
	for i, card := range cards {
		docID := seedID(kindKnowledgeDoc, org.orgByte, card.seq, 0, 0)
		docIDs[i] = docID
		created := base.AddDate(0, i, 0).Add(3 * time.Hour)
		records = append(records, seedRecord{"knowledge_documents", bson.M{
			"_id":             docID,
			"content":         card.content,
			"topics":          card.topics,
			"properties":      bson.M{"repo": repo, "language": "go"},
			"authors":         []bson.M{{"user_id": card.author.Hex(), "name": nameByID[card.author.Hex()]}},
			"created_at":      created,
			"updated_at":      created,
			"organization_id": orgID.Hex(),
		}})
	}

	// Directed learning edges: seeker != author for every row (no silent skips).
	// Locals 1/2/3 are the demo-cast trio and must all appear as seeker and author.
	type edge struct {
		seq     byte
		seeker  bson.ObjectID
		docIdx  int
		day     int
		session string
	}
	edges := []edge{
		{1, a, 0, 20, "thread-demo-1"},   // 1←2
		{2, a, 2, 35, "thread-demo-1"},   // 1←2
		{3, a, 4, 50, "thread-demo-2"},   // 1←3
		{4, a, 6, 65, "thread-demo-2"},   // 1←3
		{5, b, 1, 30, "thread-demo-3"},   // 2←1
		{6, b, 3, 55, "thread-demo-3"},   // 2←1
		{7, b, 4, 80, "thread-demo-4"},   // 2←3
		{8, b, 6, 95, "thread-demo-4"},   // 2←3
		{9, c, 0, 40, "thread-demo-5"},   // 3←2
		{10, c, 1, 70, "thread-demo-5"},  // 3←1
		{11, c, 2, 100, "thread-demo-6"}, // 3←2
		{12, c, 3, 120, "thread-demo-6"}, // 3←1
		{13, a, 5, 140, "thread-demo-7"}, // 1←2
		{14, b, 7, 160, "thread-demo-8"}, // 2←1
	}
	for _, e := range edges {
		card := cards[e.docIdx]
		if e.seeker.Hex() == card.author.Hex() {
			panic("seed knowledge edge must not be self-referential")
		}
		created := base.AddDate(0, 0, e.day).Add(11 * time.Hour)
		records = append(records, seedRecord{"knowledge_connects", bson.M{
			"_id":             seedID(kindKnowledgeConnect, org.orgByte, e.seq, 0, 0),
			"organization_id": orgID.Hex(),
			"seeker_user_id":  e.seeker.Hex(),
			"author_user_id":  card.author.Hex(),
			"document_id":     docIDs[e.docIdx].Hex(),
			"session_id":      e.session,
			"topics":          card.topics,
			"created_at":      created,
		}})
	}
	return records
}

func practiceEventsForUser(
	org orgSpec,
	orgID bson.ObjectID,
	local byte,
	userID bson.ObjectID,
	projectIDs, sessionIDs []bson.ObjectID,
	repoToProject map[string]int,
) []seedRecord {
	days := practiceDaysForUser(org, local)
	records := make([]seedRecord, 0, len(days))
	for seq, day := range days {
		file := pickFile(org.files, int(local)+seq*17)
		projIdx := repoToProject[file.repoName]
		started := day.Add(time.Duration(9+seq%7)*time.Hour + time.Duration((seq*13)%50)*time.Minute)
		outcome := outcomeFor(org, local, day, seq)
		answerMs := answerTimeMs(org, local, outcome, seq)
		totalMs := answerMs + int64(30000+((seq*1103)%90000))
		attempts := 1
		if outcome == "failed_reveal" || seq%5 == 0 {
			attempts = 2
		}
		endLine := 20 + (seq % 40)
		records = append(records, seedRecord{"practice_events", bson.M{
			"_id":                 seedEventID(org.orgByte, local, uint16(seq+1)),
			"practice":            "lead_and_reveal",
			"organization_id":     orgID,
			"user_id":             userID,
			"session_id":          sessionIDs[projIdx],
			"project_id":          projectIDs[projIdx],
			"started_at":          started,
			"ended_at":            started.Add(time.Duration(totalMs) * time.Millisecond),
			"total_duration_ms":   totalMs,
			"active_answer_time_ms": answerMs,
			"attempts":            attempts,
			"outcome":             outcome,
			"points_delta":        nil,
			"repo_name":           file.repoName,
			"repo_org":            org.repoOrg,
			"file_path":           file.filePath,
			"module":              file.module,
			"start_line":          1,
			"end_line":            endLine,
		}})
	}
	return records
}

func practiceDaysForUser(org orgSpec, local byte) []time.Time {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)

	candidates := make([]time.Time, 0, 280)
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		weekday := cursor.Weekday()
		include := false
		switch {
		case org.orgByte == orgNovaPay && local == featuredAlexLocal:
			include = weekday >= time.Monday && weekday <= time.Friday
		case org.orgByte == orgNovaPay && local == featuredJordanLocal:
			if cursor.Month() < time.April {
				include = weekday == time.Wednesday
			} else {
				include = weekday >= time.Monday && weekday <= time.Friday
			}
		case org.orgByte == orgAtlasHealth && local == featuredSamLocal:
			include = weekday != time.Sunday
		default:
			include = weekday >= time.Monday && weekday <= time.Friday
		}
		if include {
			candidates = append(candidates, cursor)
		}
	}

	var target int
	switch {
	case org.orgByte == orgNovaPay && local == featuredAlexLocal:
		target = 80
	case org.orgByte == orgNovaPay && local == featuredJordanLocal:
		target = 48
	case org.orgByte == orgAtlasHealth && local == featuredSamLocal:
		target = 90
	default:
		target = 12 + int(local)%14
	}
	if target > len(candidates) {
		target = len(candidates)
	}

	days := make([]time.Time, 0, target+target/4)
	for i := 0; i < target; i++ {
		// Spread selections evenly across the eligible calendar.
		idx := (i * len(candidates)) / target
		if idx >= len(candidates) {
			idx = len(candidates) - 1
		}
		// Small deterministic jitter so days are not perfectly uniform.
		jitter := deterministic(int(org.orgByte)*200+int(local)*13+i, candidates[idx].YearDay()) % 3
		idx = idx + jitter
		if idx >= len(candidates) {
			idx = len(candidates) - 1
		}
		day := candidates[idx]
		// Jordan: keep early-year sparse by dropping most pre-April picks beyond a few.
		if org.orgByte == orgNovaPay && local == featuredJordanLocal && day.Month() < time.April {
			if deterministic(int(local), day.YearDay())%3 != 0 {
				continue
			}
		}
		days = append(days, day)
		if len(days) < target+10 &&
			(local == featuredAlexLocal || (org.orgByte == orgAtlasHealth && local == featuredSamLocal)) &&
			deterministic(int(local)+7, day.YearDay()+i) % 4 == 0 {
			days = append(days, day)
		}
	}
	if len(days) > target {
		days = days[:target]
	}
	return days
}

func outcomeFor(org orgSpec, local byte, day time.Time, seq int) string {
	roll := deterministic(int(org.orgByte)*1000+int(local)*31+seq, day.YearDay()) % 100
	month := int(day.Month())

	switch {
	case org.orgByte == orgNovaPay && local == featuredAlexLocal:
		if roll < 85 {
			return "correct"
		}
		return "failed_reveal"
	case org.orgByte == orgNovaPay && local == featuredJordanLocal:
		// Improves over the year: ~40% early → ~80% late.
		threshold := 40 + (month-1)*5
		if threshold > 85 {
			threshold = 85
		}
		if roll < threshold {
			return "correct"
		}
		return "failed_reveal"
	case org.orgByte == orgAtlasHealth && local == featuredSamLocal:
		if roll < 55 {
			return "correct"
		}
		return "failed_reveal"
	case org.improving:
		// Org-wide improvement curve for NovaPay members.
		threshold := 48 + (month-1)*4
		if threshold > 82 {
			threshold = 82
		}
		if roll < threshold {
			return "correct"
		}
		return "failed_reveal"
	default:
		// AtlasHealth: sticky ~48–55% correct, slight decline late year.
		threshold := 55 - (month-1)/2
		if threshold < 45 {
			threshold = 45
		}
		if roll < threshold {
			return "correct"
		}
		return "failed_reveal"
	}
}

func answerTimeMs(org orgSpec, local byte, outcome string, seq int) int64 {
	base := int64(12000 + (seq*997)%18000)
	if org.orgByte == orgAtlasHealth || local == featuredSamLocal {
		base = int64(22000 + (seq*1301)%35000)
	}
	if org.orgByte == orgNovaPay && local == featuredAlexLocal {
		base = int64(8000 + (seq*701)%12000)
	}
	if outcome == "failed_reveal" {
		base += 8000
	}
	return base
}

func pickFile(files []fileTarget, salt int) fileTarget {
	total := 0
	for _, f := range files {
		total += f.weight
	}
	if total <= 0 {
		return files[0]
	}
	n := ((salt % total) + total) % total
	for _, f := range files {
		n -= f.weight
		if n < 0 {
			return f
		}
	}
	return files[len(files)-1]
}

// DETERMINISTIC MIXER — STABLE ACROSS RUNS, NO WALL-CLOCK RANDOMNESS
func deterministic(a, b int) int {
	x := uint32(a*374761393 + b*668265263)
	x = (x ^ (x >> 13)) * 1274126177
	return int(x & 0x7fffffff)
}
