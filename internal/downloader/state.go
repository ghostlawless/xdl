package downloader

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type CheckpointStatus string

const (
	CheckpointPending CheckpointStatus = "pending"
	CheckpointDone    CheckpointStatus = "done"
	CheckpointSkipped CheckpointStatus = "skipped"
	CheckpointFailed  CheckpointStatus = "failed"
)

const checkpointVersion = 1

type CheckpointItem struct {
	Index  int              `json:"index"`
	URL    string           `json:"url"`
	Type   string           `json:"type"`
	Status CheckpointStatus `json:"status"`
	Size   int64            `json:"size"`
}

type Checkpoint struct {
	Version   int              `json:"version"`
	User      string           `json:"user"`
	RunID     string           `json:"run_id"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Items     []CheckpointItem `json:"items"`
	urlIndex  map[string]int   `json:"-"`
}

func NewCheckpoint(user, runID string, medias []scraper.Media) *Checkpoint {
	t := time.Now().UTC()
	items := make([]CheckpointItem, len(medias))
	for i, m := range medias {
		items[i] = CheckpointItem{Index: i, URL: m.URL, Type: m.Type, Status: CheckpointPending}
	}
	cp := &Checkpoint{
		Version:   checkpointVersion,
		User:      user,
		RunID:     runID,
		CreatedAt: t,
		UpdatedAt: t,
		Items:     items,
	}
	cp.buildIndex()
	return cp
}

func (c *Checkpoint) buildIndex() {
	c.urlIndex = make(map[string]int, len(c.Items))
	for i, it := range c.Items {
		if it.URL != "" {
			c.urlIndex[it.URL] = i
		}
	}
}

func (c *Checkpoint) updateTimestamp() {
	c.UpdatedAt = time.Now().UTC()
}

func (c *Checkpoint) MarkByIndex(idx int, status CheckpointStatus, size int64) {
	if c == nil || idx < 0 || idx >= len(c.Items) {
		return
	}
	item := c.Items[idx]
	item.Status = status
	if size >= 0 {
		item.Size = size
	}
	c.Items[idx] = item
	c.updateTimestamp()
}

func (c *Checkpoint) MarkByURL(url string, status CheckpointStatus, size int64) {
	if c == nil || url == "" {
		return
	}
	if c.urlIndex == nil {
		c.buildIndex()
	}
	i, ok := c.urlIndex[url]
	if !ok {
		return
	}
	c.MarkByIndex(i, status, size)
}

func (c *Checkpoint) PendingItems() []CheckpointItem {
	if c == nil {
		return nil
	}
	out := make([]CheckpointItem, 0, len(c.Items))
	for _, it := range c.Items {
		if it.Status == CheckpointPending {
			out = append(out, it)
		}
	}
	return out
}

func (c *Checkpoint) CompletedCount() (done, skipped, failed int) {
	if c == nil {
		return
	}
	for _, it := range c.Items {
		switch it.Status {
		case CheckpointDone:
			done++
		case CheckpointSkipped:
			skipped++
		case CheckpointFailed:
			failed++
		}
	}
	return
}

func (c *Checkpoint) Save(path string) error {
	if c == nil {
		return errors.New("nil checkpoint")
	}
	if path == "" {
		return errors.New("empty checkpoint path")
	}
	c.updateTimestamp()
	dir := filepath.Dir(path)
	if err := utils.EnsureDir(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return utils.SaveToFile(path, data)
}

func LoadCheckpoint(path string) (*Checkpoint, error) {
	if path == "" {
		return nil, errors.New("empty checkpoint path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	if cp.Version <= 0 {
		cp.Version = checkpointVersion
	}
	cp.buildIndex()
	return &cp, nil
}
