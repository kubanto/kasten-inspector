package report

import (
	"time"

	"github.com/veeam/kasten-inspector/pkg/cluster"
	"github.com/veeam/kasten-inspector/pkg/kasten"
)

// Data is the top-level report input.
type Data struct {
	GeneratedAt time.Time            `json:"generatedAt"`
	ToolVersion string               `json:"toolVersion"`
	Author      string               `json:"author,omitempty"`
	Cluster     *cluster.Info        `json:"cluster"`
	Kasten      *kasten.Data         `json:"kasten"`
}
