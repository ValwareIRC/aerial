// Package equality reports outcome disparities from optional, self-declared
// attributes. Observations carry no identifier and cells below k are withheld.
package equality

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultK is the minimum cell size that may be published.
const DefaultK = 10

// Observation is one outcome with its declared attributes. It deliberately has
// no party, premises or case identifier: this data supports aggregate reporting
// and must not be re-joinable to a person or to an enforcement decision.
type Observation struct {
	Outcome    string
	Attributes map[string]string
}

type Cell struct {
	Value   string `json:"value"`
	Outcome string `json:"outcome"`
	Count   int    `json:"count"`
}

type Report struct {
	Attribute       string `json:"attribute"`
	K               int    `json:"k"`
	Cells           []Cell `json:"cells"`
	SuppressedCells int    `json:"suppressedCells"`
	SuppressedCount int    `json:"suppressedCount"`
	TotalCount      int    `json:"totalCount"`
}

// Rate is the share of one outcome within an attribute value, published only
// where every contributing cell clears k.
type Rate struct {
	Value      string  `json:"value"`
	Outcome    string  `json:"outcome"`
	Count      int     `json:"count"`
	Total      int     `json:"total"`
	Proportion float64 `json:"proportion"`
}

type Monitor struct {
	mu sync.Mutex
	k  int
	// counts[attribute][value][outcome]
	counts map[string]map[string]map[string]int
	total  int
}

func New(k int) (*Monitor, error) {
	if k < 2 {
		return nil, fmt.Errorf("equality: k must be at least 2, got %d", k)
	}
	return &Monitor{k: k, counts: map[string]map[string]map[string]int{}}, nil
}

func MustNew(k int) *Monitor {
	m, err := New(k)
	if err != nil {
		panic(err)
	}
	return m
}

func (m *Monitor) Observe(o Observation) error {
	if o.Outcome == "" {
		return fmt.Errorf("equality: observation needs an outcome")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	for attr, value := range o.Attributes {
		if value == "" {
			continue
		}
		if m.counts[attr] == nil {
			m.counts[attr] = map[string]map[string]int{}
		}
		if m.counts[attr][value] == nil {
			m.counts[attr][value] = map[string]int{}
		}
		m.counts[attr][value][o.Outcome]++
	}
	return nil
}

// Report returns the cells for an attribute, withholding any below k and saying
// how much was withheld.
func (m *Monitor) Report(attribute string) Report {
	m.mu.Lock()
	defer m.mu.Unlock()

	r := Report{Attribute: attribute, K: m.k, TotalCount: m.total}
	for value, outcomes := range m.counts[attribute] {
		for outcome, count := range outcomes {
			if count < m.k {
				r.SuppressedCells++
				r.SuppressedCount += count
				continue
			}
			r.Cells = append(r.Cells, Cell{Value: value, Outcome: outcome, Count: count})
		}
	}
	sort.Slice(r.Cells, func(i, j int) bool {
		if r.Cells[i].Value != r.Cells[j].Value {
			return r.Cells[i].Value < r.Cells[j].Value
		}
		return r.Cells[i].Outcome < r.Cells[j].Outcome
	})
	return r
}

// Rates gives the proportion of an outcome within each attribute value. A value
// is omitted unless its outcome cell and its total both clear k, so a rate can
// never be reconstructed from a suppressed cell.
func (m *Monitor) Rates(attribute, outcome string) []Rate {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Rate
	for value, outcomes := range m.counts[attribute] {
		total := 0
		for _, c := range outcomes {
			total += c
		}
		count := outcomes[outcome]
		if count < m.k || total < m.k {
			continue
		}
		out = append(out, Rate{
			Value: value, Outcome: outcome, Count: count, Total: total,
			Proportion: float64(count) / float64(total),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// Attributes lists the attributes observed.
func (m *Monitor) Attributes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.counts))
	for a := range m.counts {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
