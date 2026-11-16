package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultFipeBaseURL is the public FIPE v2 API used when FIPE_BASE_URL is unset.
	defaultFipeBaseURL = "https://fipe.parallelum.com.br/api/v2"
	// maxUpstreamBody caps how much of an upstream response is read (5 MiB).
	maxUpstreamBody = 5 << 20
)

// PriceResponse models the JSON returned by FIPE v2 for a vehicle price.
type PriceResponse struct {
	Price          string `json:"price"` // human-readable price, e.g. "R$ 71.229,00"
	Brand          string `json:"brand"`
	Model          string `json:"model"`
	ModelYear      int    `json:"modelYear"`
	Fuel           string `json:"fuel"`
	CodeFipe       string `json:"codeFipe"`
	ReferenceMonth string `json:"referenceMonth"`
	VehicleType    int    `json:"vehicleType"`
	FuelAcronym    string `json:"fuelAcronym"`
}

// historyEntry is one month of the FIPE v2 history endpoint.
type historyEntry struct {
	Price     string `json:"price"`
	Month     string `json:"month"`
	Reference string `json:"reference"`
}

// fipeHistory models GET /{type}/{codeFipe}/years/{yearId}/history.
type fipeHistory struct {
	PriceResponse
	PriceHistory []historyEntry `json:"priceHistory"`
}

// upstreamError reports a non-200 answer from the FIPE API.
type upstreamError struct {
	StatusCode int
	Path       string
}

func (e *upstreamError) Error() string {
	return fmt.Sprintf("FIPE API returned status %d for %s", e.StatusCode, e.Path)
}

// fipeClient talks to the FIPE v2 REST API.
type fipeClient struct {
	baseURL string
	token   string // optional X-Subscription-Token for higher rate limits
	http    *http.Client
}

func newFipeClient(baseURL, token string) *fipeClient {
	return &fipeClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second, Transport: tracedTransport(http.DefaultTransport)},
	}
}

// get performs a GET on baseURL+path and returns the response body.
// path must already contain only validated, URL-safe segments.
func (c *fipeClient) get(ctx context.Context, path string) ([]byte, error) {
	// G704: baseURL comes from operator config and every path segment is validated by idPattern.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil) //nolint:gosec
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gofipe/"+appVersion)
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("X-Subscription-Token", c.token)
	}

	resp, err := c.http.Do(req) //nolint:gosec // G704: see above
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain a little so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &upstreamError{StatusCode: resp.StatusCode, Path: path}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
}

// upstreamStatus maps an error from fipeClient to the status the BFF returns.
func upstreamStatus(err error) int {
	if ue, ok := errors.AsType[*upstreamError](err); ok {
		switch ue.StatusCode {
		case http.StatusNotFound:
			return http.StatusNotFound
		case http.StatusTooManyRequests:
			return http.StatusTooManyRequests
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

var (
	priceNumber       = regexp.MustCompile(`[0-9][0-9.,]*`)
	thousandsDotsOnly = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{3})+$`)
	errNoNumericPrice = errors.New("no numeric part in price")
)

// parseFipePrice converts FIPE price strings such as "R$ 71.229,00" to float64.
// It accepts "," or "." as the decimal separator.
func parseFipePrice(s string) (float64, error) {
	m := priceNumber.FindString(strings.TrimSpace(s))
	if m == "" {
		return math.NaN(), errNoNumericPrice
	}
	hasDot, hasComma := strings.Contains(m, "."), strings.Contains(m, ",")
	switch {
	case hasDot && hasComma:
		// Whichever separator comes last is the decimal one.
		if strings.LastIndex(m, ",") > strings.LastIndex(m, ".") {
			m = strings.ReplaceAll(m, ".", "") // 1.234,56
			m = strings.ReplaceAll(m, ",", ".")
		} else {
			m = strings.ReplaceAll(m, ",", "") // 1,234.56
		}
	case hasComma:
		m = strings.ReplaceAll(m, ",", ".") // 1234,56
	case hasDot && thousandsDotsOnly.MatchString(m):
		m = strings.ReplaceAll(m, ".", "") // 71.229 (BR thousands, no cents)
	}
	return strconv.ParseFloat(m, 64)
}
