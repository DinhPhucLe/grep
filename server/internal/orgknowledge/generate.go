package orgknowledge

import (
	"fmt"
	"strings"
	"time"
)

// GenerateSyntheticExport builds GitHub-shaped rows for pitch demos without Marketplace access.
// Output is deterministic for a given count.
func GenerateSyntheticExport(count int) []GitHubExportRow {
	if count < 0 {
		count = 0
	}
	templates := syntheticTemplates()
	repos := []string{
		"novapay/novapay-api", "novapay/novapay-web", "novapay/novapay-cli",
		"atlashealth/atlas-core", "atlashealth/atlas-billing", "atlashealth/atlas-portal",
	}
	actors := []string{"alexr", "jordank", "caseyn", "rileyp", "samok", "ninav", "priyash", "marcusb"}
	types := []string{"IssuesEvent", "PullRequestEvent", "IssueCommentEvent"}
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	out := make([]GitHubExportRow, 0, count)
	for i := 0; i < count; i++ {
		tpl := templates[i%len(templates)]
		repo := repos[i%len(repos)]
		// Bias payments/health wording toward matching org buckets in MapExportRows.
		title := fmt.Sprintf("%s (%s #%d)", tpl.title, shortRepo(repo), i+1)
		body := fmt.Sprintf("%s\n\nContext: repo %s module discussion for demo corpus item %d.", tpl.body, repo, i+1)
		labels := append([]string{}, tpl.labels...)
		if strings.Contains(repo, "atlas") {
			labels = append(labels, "atlashealth")
		} else {
			labels = append(labels, "novapay")
		}
		created := base.Add(time.Duration(i) * 3 * time.Hour)
		out = append(out, GitHubExportRow{
			EventID:    fmt.Sprintf("synth-%05d", i+1),
			EventType:  types[i%len(types)],
			RepoName:   repo,
			ActorLogin: actors[i%len(actors)],
			Title:      title,
			Body:       body,
			CreatedAt:  created.Format(time.RFC3339),
			Labels:     labels,
		})
	}
	return out
}

func shortRepo(repo string) string {
	parts := strings.Split(repo, "/")
	if len(parts) == 0 {
		return repo
	}
	return parts[len(parts)-1]
}

type synthTemplate struct {
	title  string
	body   string
	labels []string
}

func syntheticTemplates() []synthTemplate {
	return []synthTemplate{
		{
			title:  "Payment retries need jitter on card network 429",
			body:   "Use exponential backoff with jitter on 429/503. Cap at five attempts. Do not retry HTTP 400 validation errors from the processor.",
			labels: []string{"payments", "retries", "reliability"},
		},
		{
			title:  "Stripe webhook HMAC must use raw body bytes",
			body:   "Verify HMAC-SHA256 over the raw request body with the endpoint secret. Reject timestamp skew over five minutes without logging the secret.",
			labels: []string{"webhooks", "security", "stripe"},
		},
		{
			title:  "FHIR patient search must scope by organizationId",
			body:   "Always filter by organizationId before name match. Use case-insensitive collation for partial names. Never return cross-tenant patient IDs.",
			labels: []string{"fhir", "privacy", "patients"},
		},
		{
			title:  "Invoice adjustment race on concurrent payouts",
			body:   "Serialize billing adjustments per member_id using a conditional version update so two workers cannot double-apply a credit.",
			labels: []string{"billing", "concurrency"},
		},
		{
			title:  "Checkout React Query staleTime causes flicker",
			body:   "Set staleTime to 30s for checkout aggregates. Invalidate only after confirmed capture. Disable refetchOnWindowFocus on kiosk pages.",
			labels: []string{"frontend", "caching", "checkout"},
		},
		{
			title:  "Claims treemap root intensity must stay zero",
			body:   "Root node intensity stays 0 as a neutral container. Sibling max drives leaf tiers 1-4 with grey, mint, amber, light red, hard red.",
			labels: []string{"dashboard", "visualization"},
		},
		{
			title:  "CLI transfers require idempotency keys",
			body:   "Send X-Idempotency-Key on every transfer POST and reuse it for retries within ten minutes so ledger entries stay unique.",
			labels: []string{"cli", "idempotency", "transfers"},
		},
		{
			title:  "Eligibility cache misses mid-cycle plan changes",
			body:   "Invalidate eligibility keys on plan_version bumps. TTL alone is insufficient when benefits change mid-cycle in the legacy eligibility path.",
			labels: []string{"eligibility", "cache", "legacy"},
		},
		{
			title:  "OAuth session rotation drops refresh tokens",
			body:   "Rotate refresh tokens atomically: write the new token before invalidating the old one, and tolerate a one-time replay window.",
			labels: []string{"auth", "oauth", "sessions"},
		},
		{
			title:  "Mongo migration order for practice collections",
			body:   "Apply users, projects, sessions, evaluation, then practice. Never skip a numbered migration when applying to Atlas.",
			labels: []string{"mongodb", "migrations", "ops"},
		},
		{
			title:  "Ledger posting must be append-only",
			body:   "Never update historical ledger rows. Post compensating entries for corrections and include the original entry id in properties.",
			labels: []string{"payments", "ledger"},
		},
		{
			title:  "Member claims UI must not fetch full PDF inline",
			body:   "Load claim metadata first, then stream PDFs from a signed URL with a short TTL. Avoid embedding multi-megabyte blobs in the portal bundle.",
			labels: []string{"portal", "claims", "performance"},
		},
	}
}
