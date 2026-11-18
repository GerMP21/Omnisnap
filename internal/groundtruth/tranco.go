package groundtruth

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Ranked is one Tranco row.
type Ranked struct {
	Rank   int
	Domain string
}

// FetchTranco downloads the public Tranco top-1M list and returns the first
// scan ranks. The list is a zip with one CSV of "rank,domain" rows.
// Source: https://tranco-list.eu
func FetchTranco(ctx context.Context, scan int) ([]Ranked, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://tranco-list.eu/top-1m.csv.zip", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0 Safari/537.36")
	client := &http.Client{Timeout: 5 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groundtruth: tranco: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("groundtruth: tranco: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("groundtruth: tranco: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("groundtruth: tranco zip: %w", err)
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("groundtruth: tranco zip is empty")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("groundtruth: tranco zip: %w", err)
	}
	defer func() { _ = rc.Close() }()
	rd := csv.NewReader(rc)
	rd.FieldsPerRecord = -1
	var out []Ranked
	for len(out) < scan {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) != 2 {
			continue
		}
		rank, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			continue
		}
		out = append(out, Ranked{Rank: rank, Domain: strings.ToLower(strings.TrimSpace(rec[1]))})
	}
	return out, nil
}
