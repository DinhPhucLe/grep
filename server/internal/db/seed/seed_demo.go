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
