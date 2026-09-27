package orgknowledge

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxTopicNetworkNodes = 40
	maxTopicNetworkLinks = 80
)

// EmployeeKnowledgeSubject scopes a person knowledge dashboard.
type EmployeeKnowledgeSubject struct {
	UserID         string `json:"userId"`
	UserName       string `json:"userName,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
	Year           int    `json:"year"`
}

// ActivityCalendar mirrors practice calendar cells (in+out count per day).
type ActivityCalendar struct {
	Status string        `json:"status"`
	Days   []CalendarDay `json:"days"`
}

// CalendarDay is one day cell.
type CalendarDay struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	Intensity int    `json:"intensity"`
}

// MetricDisplay is the card primary/secondary text.
type MetricDisplay struct {
	Primary   string  `json:"primary"`
	Secondary *string `json:"secondary,omitempty"`
}

// RatioSegment is a labeled ratio part.
type RatioSegment struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// Metric matches dashboard DashboardMetric (stat or ratio).
type Metric struct {
	ID            string        `json:"id"`
	Category      string        `json:"category"`
	Label         string        `json:"label"`
	Description   *string       `json:"description,omitempty"`
	Status        string        `json:"status"`
	Display       MetricDisplay `json:"display"`
	Visualization any           `json:"visualization"`
}

// Summary is optional prose bullets.
type Summary struct {
	Status  string   `json:"status"`
	Bullets []string `json:"bullets"`
}

// EmployeeKnowledgeView is GET /api/v1/dashboard/people/{id}/knowledge.
type EmployeeKnowledgeView struct {
	SchemaVersion    string                   `json:"schemaVersion"`
	GeneratedAt      string                   `json:"generatedAt"`
	Subject          EmployeeKnowledgeSubject `json:"subject"`
	ActivityCalendar ActivityCalendar         `json:"activityCalendar"`
	Metrics          []Metric                 `json:"metrics"`
	Summary          *Summary                 `json:"summary,omitempty"`
}

// OrgKnowledgeSubject scopes an org knowledge dashboard.
type OrgKnowledgeSubject struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName,omitempty"`
	From             string `json:"from"`
	To               string `json:"to"`
}

// TopicNode is a topic in the org co-occurrence network.
type TopicNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
}

// TopicLink is an undirected co-occurrence edge between topics.
type TopicLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Weight int    `json:"weight"`
}

// TopicNetwork is topics-as-nodes for org viz.
type TopicNetwork struct {
	Status string      `json:"status"`
	Nodes  []TopicNode `json:"nodes"`
	Links  []TopicLink `json:"links"`
}

// LearningDot is one connected knowledge document.
type LearningDot struct {
	ID           string    `json:"id"`
	Topics       []string  `json:"topics"`
	AuthorUserID string    `json:"authorUserId"`
	AuthorName   string    `json:"authorName,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// LearningDots is the org document-dot series.
type LearningDots struct {
	Status string        `json:"status"`
	Items  []LearningDot `json:"items"`
}

// OrgKnowledgeView is GET /api/v1/dashboard/organizations/{id}/knowledge.
type OrgKnowledgeView struct {
	SchemaVersion string              `json:"schemaVersion"`
	GeneratedAt   string              `json:"generatedAt"`
	Subject       OrgKnowledgeSubject `json:"subject"`
	TopicNetwork  TopicNetwork        `json:"topicNetwork"`
	LearningDots  LearningDots        `json:"learningDots"`
	Metrics       []Metric            `json:"metrics"`
	Summary       *Summary            `json:"summary,omitempty"`
}

// BuildEmployeeKnowledgeView aggregates connect edges for one person/year.
func BuildEmployeeKnowledgeView(subject EmployeeKnowledgeSubject, edges []Connect) EmployeeKnowledgeView {
	view := EmployeeKnowledgeView{
		SchemaVersion: "employee_knowledge.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Subject:       subject,
		Metrics:       []Metric{},
	}
	if len(edges) == 0 {
		view.ActivityCalendar = ActivityCalendar{Status: "no_data", Days: []CalendarDay{}}
		view.Metrics = emptyEmployeeEdgeMetrics()
		return view
	}

	orgID := subject.OrganizationID
	if orgID == "" {
		orgID = edges[0].OrganizationID
		view.Subject.OrganizationID = orgID
	}

	dayCounts := map[string]int{}
	out, in := 0, 0
	sessions := map[string]struct{}{}
	for _, e := range edges {
		day := e.CreatedAt.UTC().Format("2006-01-02")
		incident := false
		if e.AuthorUserID == subject.UserID {
			out++
			incident = true
		}
		if e.SeekerUserID == subject.UserID {
			in++
			incident = true
		}
		if incident {
			dayCounts[day]++
			if e.SessionID != "" {
				sessions[e.SessionID] = struct{}{}
			}
		}
	}

	maxCount := 0
	for _, c := range dayCounts {
		if c > maxCount {
			maxCount = c
		}
	}
	days := make([]CalendarDay, 0, len(dayCounts))
	for date, count := range dayCounts {
		days = append(days, CalendarDay{
			Date:      date,
			Count:     count,
			Intensity: intensityBucket(count, maxCount),
		})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	view.ActivityCalendar = ActivityCalendar{Status: "available", Days: days}

	total := in + out
	ratioValue := 0.0
	if total > 0 {
		ratioValue = float64(in) / float64(total)
	}
	avgPerSession := 0.0
	if len(sessions) > 0 {
		avgPerSession = float64(total) / float64(len(sessions))
	}

	view.Metrics = []Metric{
		statMetric("edges_out", "significant", "Edges sent", strconv.Itoa(out)),
		statMetric("edges_in", "significant", "Edges received", strconv.Itoa(in)),
		ratioMetric("in_out_ratio", "significant", "In / out mix", ratioValue, in, out),
		statMetric("avg_edges_per_session", "descriptive", "Avg edges / session", formatFloat(avgPerSession)),
	}
	view.Summary = &Summary{
		Status: "available",
		Bullets: []string{
			fmt.Sprintf("%d learning edges in %d (%d in, %d out)", total, subject.Year, in, out),
		},
	}
	return view
}

// BuildOrgKnowledgeView aggregates org connects into metrics, topic graph, and document dots.
func BuildOrgKnowledgeView(subject OrgKnowledgeSubject, edges []Connect, docs []Document) OrgKnowledgeView {
	view := OrgKnowledgeView{
		SchemaVersion: "org_knowledge.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Subject:       subject,
		Metrics:       []Metric{},
		TopicNetwork:  TopicNetwork{Status: "no_data", Nodes: []TopicNode{}, Links: []TopicLink{}},
		LearningDots:  LearningDots{Status: "no_data", Items: []LearningDot{}},
	}
	if len(edges) == 0 {
		return view
	}

	seekers := map[string]struct{}{}
	teachers := map[string]struct{}{}
	docsTouched := map[string]struct{}{}
	sessionEdgeCounts := map[string]int{}
	topicWeight := map[string]int{}
	linkWeight := map[string]int{}

	for _, e := range edges {
		seekers[e.SeekerUserID] = struct{}{}
		teachers[e.AuthorUserID] = struct{}{}
		docsTouched[e.DocumentID] = struct{}{}
		if e.SessionID != "" {
			sessionEdgeCounts[e.SessionID]++
		}
		topics := uniqueTopics(e.Topics)
		for _, t := range topics {
			topicWeight[t]++
		}
		for i := 0; i < len(topics); i++ {
			for j := i + 1; j < len(topics); j++ {
				a, b := topics[i], topics[j]
				if a > b {
					a, b = b, a
				}
				key := a + "\x00" + b
				linkWeight[key]++
			}
		}
	}

	topTopics := topKeyed(topicWeight, maxTopicNetworkNodes)
	allowed := map[string]struct{}{}
	nodes := make([]TopicNode, 0, len(topTopics))
	for _, t := range topTopics {
		allowed[t.key] = struct{}{}
		nodes = append(nodes, TopicNode{ID: t.key, Label: t.key, Weight: t.value})
	}
	type linkRow struct {
		source, target string
		weight         int
	}
	var links []linkRow
	for key, w := range linkWeight {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		if _, ok := allowed[parts[0]]; !ok {
			continue
		}
		if _, ok := allowed[parts[1]]; !ok {
			continue
		}
		links = append(links, linkRow{source: parts[0], target: parts[1], weight: w})
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].weight != links[j].weight {
			return links[i].weight > links[j].weight
		}
		if links[i].source != links[j].source {
			return links[i].source < links[j].source
		}
		return links[i].target < links[j].target
	})
	if len(links) > maxTopicNetworkLinks {
		links = links[:maxTopicNetworkLinks]
	}
	outLinks := make([]TopicLink, 0, len(links))
	for _, l := range links {
		outLinks = append(outLinks, TopicLink{Source: l.source, Target: l.target, Weight: l.weight})
	}
	view.TopicNetwork = TopicNetwork{Status: "available", Nodes: nodes, Links: outLinks}

	docByID := map[string]Document{}
	for _, d := range docs {
		docByID[d.ID.Hex()] = d
	}
	// First-seen metadata from edges when document lookup misses.
	firstEdge := map[string]Connect{}
	for _, e := range edges {
		if _, ok := firstEdge[e.DocumentID]; !ok {
			firstEdge[e.DocumentID] = e
		}
	}
	ids := make([]string, 0, len(docsTouched))
	for id := range docsTouched {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]LearningDot, 0, len(ids))
	for _, id := range ids {
		if doc, ok := docByID[id]; ok {
			authorID, authorName := primaryAuthor(doc)
			items = append(items, LearningDot{
				ID:           id,
				Topics:       append([]string(nil), doc.Topics...),
				AuthorUserID: authorID,
				AuthorName:   authorName,
				CreatedAt:    doc.CreatedAt.UTC(),
			})
			continue
		}
		e := firstEdge[id]
		items = append(items, LearningDot{
			ID:           id,
			Topics:       append([]string(nil), e.Topics...),
			AuthorUserID: e.AuthorUserID,
			CreatedAt:    e.CreatedAt.UTC(),
		})
	}
	view.LearningDots = LearningDots{Status: "available", Items: items}

	sessionCounts := make([]int, 0, len(sessionEdgeCounts))
	for _, c := range sessionEdgeCounts {
		sessionCounts = append(sessionCounts, c)
	}
	medianSession := medianInt(sessionCounts)

	view.Metrics = []Metric{
		statMetric("total_connects", "significant", "Total connects", strconv.Itoa(len(edges))),
		statMetric("unique_seekers", "significant", "Unique seekers", strconv.Itoa(len(seekers))),
		statMetric("unique_teachers", "descriptive", "Unique teachers", strconv.Itoa(len(teachers))),
		statMetric("docs_connected", "descriptive", "Docs connected", strconv.Itoa(len(docsTouched))),
		statMetric("median_edges_per_session", "descriptive", "Median edges / session", strconv.Itoa(medianSession)),
	}
	view.Summary = &Summary{
		Status: "available",
		Bullets: []string{
			fmt.Sprintf("%d connects across %d documents", len(edges), len(docsTouched)),
		},
	}
	return view
}

func emptyEmployeeEdgeMetrics() []Metric {
	return []Metric{
		statMetric("edges_out", "significant", "Edges sent", "0"),
		statMetric("edges_in", "significant", "Edges received", "0"),
		ratioMetric("in_out_ratio", "significant", "In / out mix", 0, 0, 0),
		statMetric("avg_edges_per_session", "descriptive", "Avg edges / session", "0"),
	}
}

func statMetric(id, category, label, primary string) Metric {
	return Metric{
		ID:       id,
		Category: category,
		Label:    label,
		Status:   "available",
		Display:  MetricDisplay{Primary: primary},
		Visualization: map[string]any{
			"type": "stat",
		},
	}
}

func ratioMetric(id, category, label string, value float64, in, out int) Metric {
	primary := formatPercent(value)
	return Metric{
		ID:       id,
		Category: category,
		Label:    label,
		Status:   "available",
		Display:  MetricDisplay{Primary: primary},
		Visualization: map[string]any{
			"type":    "ratio",
			"value":   value,
			"minimum": 0,
			"maximum": 1,
			"segments": []RatioSegment{
				{Label: "In", Value: in},
				{Label: "Out", Value: out},
			},
		},
	}
}

func formatPercent(v float64) string {
	return fmt.Sprintf("%.0f%%", v*100)
}

func formatFloat(v float64) string {
	if v == float64(int(v)) {
		return strconv.Itoa(int(v))
	}
	return fmt.Sprintf("%.1f", v)
}

func intensityBucket(count, maxCount int) int {
	if count <= 0 || maxCount <= 0 {
		return 0
	}
	if maxCount == 1 {
		return 1
	}
	level := 1 + (count*4)/maxCount
	if level > 4 {
		level = 4
	}
	if count > 0 && level < 1 {
		level = 1
	}
	return level
}

func uniqueTopics(topics []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(topics))
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

type keyedCount struct {
	key   string
	value int
}

func topKeyed(m map[string]int, limit int) []keyedCount {
	rows := make([]keyedCount, 0, len(m))
	for k, v := range m {
		rows = append(rows, keyedCount{key: k, value: v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].value != rows[j].value {
			return rows[i].value > rows[j].value
		}
		return rows[i].key < rows[j].key
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func primaryAuthor(doc Document) (string, string) {
	if len(doc.Authors) == 0 {
		return "", ""
	}
	return doc.Authors[0].UserID, doc.Authors[0].Name
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	cp := append([]int(nil), values...)
	sort.Ints(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 0 {
		return (cp[mid-1] + cp[mid]) / 2
	}
	return cp[mid]
}
