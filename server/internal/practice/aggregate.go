package practice

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

type EmployeeSubject struct {
	UserID         string `json:"userId"`
	UserName       string `json:"userName,omitempty"`
	OrganizationID string `json:"organizationId"`
	Practice       string `json:"practice"`
	Year           int    `json:"year"`
}

type CalendarDay struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	Intensity int    `json:"intensity"`
}

type ActivityCalendar struct {
	Status string        `json:"status"`
	Days   []CalendarDay `json:"days"`
}

type PieSegment struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type OutcomePie struct {
	Status   string       `json:"status"`
	Segments []PieSegment `json:"segments"`
	Display  struct {
		Primary   string  `json:"primary"`
		Secondary *string `json:"secondary,omitempty"`
	} `json:"display"`
}

type MetricDisplay struct {
	Primary   string  `json:"primary"`
	Secondary *string `json:"secondary,omitempty"`
}

type Metric struct {
	ID            string        `json:"id"`
	Category      string        `json:"category"`
	Label         string        `json:"label"`
	Description   *string       `json:"description,omitempty"`
	Status        string        `json:"status"`
	Display       MetricDisplay `json:"display"`
	Visualization struct {
		Type string `json:"type"`
	} `json:"visualization"`
}

type Summary struct {
	Status  string   `json:"status"`
	Bullets []string `json:"bullets"`
}

type EmployeePracticeView struct {
	SchemaVersion    string           `json:"schemaVersion"`
	GeneratedAt      string           `json:"generatedAt"`
	Subject          EmployeeSubject  `json:"subject"`
	ActivityCalendar ActivityCalendar `json:"activityCalendar"`
	OutcomePie       OutcomePie       `json:"outcomePie"`
	Metrics          []Metric         `json:"metrics"`
	Summary          *Summary         `json:"summary,omitempty"`
}

type OrgSubject struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName,omitempty"`
	Practice         string `json:"practice"`
	From             string `json:"from"`
	To               string `json:"to"`
}

type TreemapNode struct {
	Name      string        `json:"name"`
	Value     int           `json:"value"`
	Intensity int           `json:"intensity"`
	Children  []TreemapNode `json:"children,omitempty"`
}

type CodebaseTreemap struct {
	ProjectID string      `json:"projectId"`
	RepoName  string      `json:"repoName"`
	Status    string      `json:"status"`
	Root      TreemapNode `json:"root"`
}

type TimeseriesPoint struct {
	T                         string `json:"t"`
	Correct                   int    `json:"correct"`
	FailedReveal              int    `json:"failedReveal"`
	MedianActiveAnswerTimeMs  *int64 `json:"medianActiveAnswerTimeMs"`
}

type Timeseries struct {
	Status string            `json:"status"`
	Points []TimeseriesPoint `json:"points"`
}

type OrgPracticeView struct {
	SchemaVersion    string            `json:"schemaVersion"`
	GeneratedAt      string            `json:"generatedAt"`
	Subject          OrgSubject        `json:"subject"`
	CodebaseTreemaps []CodebaseTreemap `json:"codebaseTreemaps"`
	Timeseries       Timeseries        `json:"timeseries"`
	Metrics          []Metric          `json:"metrics"`
	Summary          *Summary          `json:"summary,omitempty"`
}

func BuildEmployeeView(subject EmployeeSubject, events []Event) EmployeePracticeView {
	view := EmployeePracticeView{
		SchemaVersion: "employee_practice.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Subject:       subject,
		Metrics:       []Metric{},
	}
	if len(events) == 0 {
		view.ActivityCalendar = ActivityCalendar{Status: "no_data", Days: []CalendarDay{}}
		view.OutcomePie = OutcomePie{Status: "no_data", Segments: []PieSegment{}}
		view.OutcomePie.Display.Primary = "—"
		return view
	}

	counts := map[string]int{}
	correct, failed := 0, 0
	actives := make([]int64, 0, len(events))
	for _, e := range events {
		day := e.StartedAt.UTC().Format("2006-01-02")
		counts[day]++
		actives = append(actives, e.ActiveAnswerTimeMs)
		switch e.Outcome {
		case OutcomeCorrect:
			correct++
		case OutcomeFailedReveal:
			failed++
		}
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	days := make([]CalendarDay, 0, len(counts))
	for date, count := range counts {
		days = append(days, CalendarDay{
			Date:      date,
			Count:     count,
			Intensity: intensityBucket(count, maxCount),
		})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	view.ActivityCalendar = ActivityCalendar{Status: "available", Days: days}

	view.OutcomePie = OutcomePie{
		Status: "available",
		Segments: []PieSegment{
			{Label: OutcomeCorrect, Value: correct},
			{Label: OutcomeFailedReveal, Value: failed},
		},
	}
	primary := fmt.Sprintf("%d correct / %d failed", correct, failed)
	view.OutcomePie.Display.Primary = primary

	median := medianInt64(actives)
	view.Metrics = []Metric{
		statMetric("total_instances", "significant", "Practice instances", strconv.Itoa(len(events))),
		statMetric("median_active_answer_ms", "significant", "Median active answer time", formatMs(median)),
		statMetric("correct_count", "descriptive", "Correct outcomes", strconv.Itoa(correct)),
		statMetric("failed_reveal_count", "descriptive", "Failed reveal outcomes", strconv.Itoa(failed)),
	}
	view.Summary = &Summary{
		Status:  "available",
		Bullets: []string{fmt.Sprintf("%d %s practice instances in %d", len(events), subject.Practice, subject.Year)},
	}
	return view
}

func BuildOrgView(subject OrgSubject, events []Event) OrgPracticeView {
	view := OrgPracticeView{
		SchemaVersion:    "org_practice.v1",
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Subject:          subject,
		CodebaseTreemaps: []CodebaseTreemap{},
		Metrics:          []Metric{},
	}
	if len(events) == 0 {
		view.Timeseries = Timeseries{Status: "no_data", Points: []TimeseriesPoint{}}
		return view
	}

	type repoKey struct {
		projectID string
		repoName  string
	}
	repos := map[repoKey]map[string]map[string]int{}
	for _, e := range events {
		key := repoKey{projectID: e.ProjectID.Hex(), repoName: e.RepoName}
		if repos[key] == nil {
			repos[key] = map[string]map[string]int{}
		}
		if repos[key][e.Module] == nil {
			repos[key][e.Module] = map[string]int{}
		}
		repos[key][e.Module][e.FilePath]++
	}
	for key, modules := range repos {
		root := TreemapNode{Name: key.repoName}
		moduleMax := 0
		for module, files := range modules {
			child := TreemapNode{Name: module}
			fileMax := 0
			for file, count := range files {
				if count > fileMax {
					fileMax = count
				}
				child.Children = append(child.Children, TreemapNode{
					Name:      file,
					Value:     count,
					Intensity: intensityBucket(count, count), // set after fileMax known
				})
				child.Value += count
			}
			for i := range child.Children {
				child.Children[i].Intensity = intensityBucket(child.Children[i].Value, fileMax)
			}
			sort.Slice(child.Children, func(i, j int) bool { return child.Children[i].Name < child.Children[j].Name })
			if child.Value > moduleMax {
				moduleMax = child.Value
			}
			root.Children = append(root.Children, child)
			root.Value += child.Value
		}
		for i := range root.Children {
			root.Children[i].Intensity = intensityBucket(root.Children[i].Value, moduleMax)
		}
		sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Name < root.Children[j].Name })
		// Root is a container; keep neutral so sibling tiers read on the frontend.
		root.Intensity = 0
		view.CodebaseTreemaps = append(view.CodebaseTreemaps, CodebaseTreemap{
			ProjectID: key.projectID,
			RepoName:  key.repoName,
			Status:    "available",
			Root:      root,
		})
	}
	sort.Slice(view.CodebaseTreemaps, func(i, j int) bool {
		return view.CodebaseTreemaps[i].RepoName < view.CodebaseTreemaps[j].RepoName
	})

	byDay := map[string]*TimeseriesPoint{}
	activesByDay := map[string][]int64{}
	for _, e := range events {
		day := e.StartedAt.UTC().Format("2006-01-02")
		p := byDay[day]
		if p == nil {
			p = &TimeseriesPoint{T: day}
			byDay[day] = p
		}
		switch e.Outcome {
		case OutcomeCorrect:
			p.Correct++
		case OutcomeFailedReveal:
			p.FailedReveal++
		}
		activesByDay[day] = append(activesByDay[day], e.ActiveAnswerTimeMs)
	}
	points := make([]TimeseriesPoint, 0, len(byDay))
	for day, p := range byDay {
		median := medianInt64(activesByDay[day])
		p.MedianActiveAnswerTimeMs = &median
		points = append(points, *p)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].T < points[j].T })
	view.Timeseries = Timeseries{Status: "available", Points: points}

	correct, failed := 0, 0
	for _, e := range events {
		if e.Outcome == OutcomeCorrect {
			correct++
		} else if e.Outcome == OutcomeFailedReveal {
			failed++
		}
	}
	view.Metrics = []Metric{
		statMetric("total_instances", "significant", "Practice instances", strconv.Itoa(len(events))),
		statMetric("correct_count", "descriptive", "Correct outcomes", strconv.Itoa(correct)),
		statMetric("failed_reveal_count", "descriptive", "Failed reveal outcomes", strconv.Itoa(failed)),
	}
	view.Summary = &Summary{
		Status:  "available",
		Bullets: []string{fmt.Sprintf("%d %s instances across %d codebases", len(events), subject.Practice, len(view.CodebaseTreemaps))},
	}
	return view
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

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]int64(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	mid := len(cp) / 2
	if len(cp)%2 == 0 {
		return (cp[mid-1] + cp[mid]) / 2
	}
	return cp[mid]
}

func formatMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func statMetric(id, category, label, primary string) Metric {
	m := Metric{
		ID:       id,
		Category: category,
		Label:    label,
		Status:   "available",
		Display:  MetricDisplay{Primary: primary},
	}
	m.Visualization.Type = "stat"
	return m
}
