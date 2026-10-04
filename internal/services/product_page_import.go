package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ProductPageDraft contains business data only. Importing a page never downloads
// images, logs into a shop, changes its cart or creates warehouse records.
type ProductPageDraft struct {
	Name                   string            `json:"name"`
	Description            string            `json:"description"`
	Manufacturer           string            `json:"manufacturer"`
	Brand                  string            `json:"brand"`
	Model                  string            `json:"model"`
	ManufacturerPartNumber string            `json:"manufacturer_part_number"`
	EAN                    string            `json:"ean"`
	Weight                 *float64          `json:"weight,omitempty"`
	Width                  *float64          `json:"width,omitempty"`
	Height                 *float64          `json:"height,omitempty"`
	Depth                  *float64          `json:"depth,omitempty"`
	Attributes             map[string]string `json:"attributes"`
	SourceURL              string            `json:"source_url"`
}

func publicProductPageIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "2001::/32", "2002::/16"} {
		_, network, _ := net.ParseCIDR(prefix)
		if network.Contains(ip) {
			return false
		}
	}
	if ip.To4() == nil {
		_, global, _ := net.ParseCIDR("2000::/3")
		return global.Contains(ip)
	}
	return true
}

func resolveProductPageHost(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("Product page host could not be resolved")
	}
	for _, ip := range ips {
		if !publicProductPageIP(ip.IP) {
			return nil, errors.New("Local, private and internal product page targets are forbidden")
		}
	}
	return ips, nil
}

func validateProductPageURL(target *url.URL) error {
	if target == nil || (target.Scheme != "https" && target.Scheme != "http") || target.Hostname() == "" || target.User != nil || len(target.String()) > 2048 {
		return errors.New("A bounded public HTTP(S) product URL without credentials is required")
	}
	if port := target.Port(); port != "" && !((target.Scheme == "https" && port == "443") || (target.Scheme == "http" && port == "80")) {
		return errors.New("Product pages must use the standard HTTP(S) port")
	}
	return nil
}

func ExtractPublicProductPage(ctx context.Context, rawURL string) (ProductPageDraft, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ProductPageDraft{}, errors.New("Invalid product URL")
	}
	if err = validateProductPageURL(target); err != nil {
		return ProductPageDraft{}, err
	}
	if _, err = resolveProductPageHost(ctx, target.Hostname()); err != nil {
		return ProductPageDraft{}, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, DisableCompression: false, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolveProductPageHost(ctx, host)
			if err != nil {
				return nil, err
			}
			// Pin the actual socket to a just-validated public address. No second
			// hostname lookup or environment proxy can bypass target validation.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("Too many product page redirects")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("Product page TLS downgrade forbidden")
		}
		if err := validateProductPageURL(req.URL); err != nil {
			return err
		}
		_, err := resolveProductPageHost(req.Context(), req.URL.Hostname())
		return err
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return ProductPageDraft{}, err
	}
	req.Header.Set("User-Agent", "WarehouseCore Product Import/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := client.Do(req)
	if err != nil {
		return ProductPageDraft{}, errors.New("Public product page could not be loaded")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ProductPageDraft{}, fmt.Errorf("Product page returned HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
		return ProductPageDraft{}, errors.New("Product URL must return HTML")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return ProductPageDraft{}, errors.New("Product page exceeds the 2 MiB import limit")
	}
	return ParseProductPage(strings.NewReader(string(body)), response.Request.URL.String())
}

func productPageString(value any, limit int) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) > limit {
		return ""
	}
	return s
}

func productPageName(value any) string {
	if m, ok := value.(map[string]any); ok {
		return productPageString(m["name"], 255)
	}
	return productPageString(value, 255)
}

func productPageMeasure(value any, weight bool) *float64 {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	v, ok := m["value"].(float64)
	if !ok {
		if s, yes := m["value"].(string); yes {
			var err error
			v, err = strconv.ParseFloat(s, 64)
			ok = err == nil
		}
	}
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	unit := strings.ToLower(productPageString(m["unitCode"], 32))
	if unit == "" {
		unit = strings.ToLower(productPageString(m["unitText"], 32))
	}
	factors := map[string]float64{"kg": 1, "kgm": 1, "g": 0.001, "grm": 0.001}
	if !weight {
		factors = map[string]float64{"cm": 1, "cmt": 1, "mm": 0.1, "mmt": 0.1, "m": 100, "mtr": 100}
	}
	factor, ok := factors[unit]
	if !ok || v*factor > 99999999.99 {
		return nil
	}
	v *= factor
	return &v
}

// ParseProductPage extracts bounded Schema.org Product data and ordinary title
// metadata. It never executes scripts or follows links found in page contents.
func ParseProductPage(reader io.Reader, sourceURL string) (ProductPageDraft, error) {
	draft := ProductPageDraft{Attributes: map[string]string{}, SourceURL: sourceURL}
	tokenizer := html.NewTokenizer(io.LimitReader(reader, (2<<20)+1))
	meta := map[string]string{}
	products := []map[string]any{}
	var collect func(any, int)
	collect = func(value any, depth int) {
		if depth > 12 || len(products) > 1 {
			return
		}
		switch v := value.(type) {
		case []any:
			for _, child := range v {
				collect(child, depth+1)
			}
		case map[string]any:
			types := []any{v["@type"]}
			if list, ok := v["@type"].([]any); ok {
				types = list
			}
			for _, kind := range types {
				if name, ok := kind.(string); ok && (name == "Product" || name == "https://schema.org/Product" || name == "http://schema.org/Product") {
					products = append(products, v)
					return
				}
			}
			if graph, ok := v["@graph"]; ok {
				collect(graph, depth+1)
			}
		}
	}
	for {
		typeToken := tokenizer.Next()
		if typeToken == html.ErrorToken {
			if err := tokenizer.Err(); err != io.EOF {
				return draft, errors.New("Invalid product HTML")
			}
			break
		}
		if typeToken != html.StartTagToken && typeToken != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data == "meta" {
			key, content := "", ""
			for _, a := range token.Attr {
				if a.Key == "property" || a.Key == "name" || a.Key == "itemprop" {
					key = strings.ToLower(a.Val)
				}
				if a.Key == "content" {
					content = a.Val
				}
			}
			if key != "" && meta[key] == "" && len(content) <= 16000 {
				meta[key] = strings.TrimSpace(content)
			}
		}
		if token.Data == "script" {
			isJSON := false
			for _, a := range token.Attr {
				if a.Key == "type" && strings.EqualFold(a.Val, "application/ld+json") {
					isJSON = true
				}
			}
			if isJSON && tokenizer.Next() == html.TextToken {
				raw := tokenizer.Text()
				if len(raw) <= 128<<10 {
					var value any
					if json.Unmarshal(raw, &value) == nil {
						collect(value, 0)
					}
				}
			}
		}
	}
	if len(products) > 1 {
		return draft, errors.New("Multiple products found; use a page for one specific product")
	}
	if len(products) == 1 {
		p := products[0]
		draft.Name = productPageString(p["name"], 255)
		draft.Description = productPageString(p["description"], 16000)
		draft.Manufacturer = productPageName(p["manufacturer"])
		draft.Brand = productPageName(p["brand"])
		draft.Model = productPageString(p["model"], 255)
		draft.ManufacturerPartNumber = productPageString(p["mpn"], 255)
		for _, key := range []string{"gtin13", "gtin14", "gtin12", "gtin8", "gtin"} {
			if draft.EAN == "" {
				draft.EAN = productPageString(p[key], 255)
			}
		}
		draft.Weight = productPageMeasure(p["weight"], true)
		draft.Width = productPageMeasure(p["width"], false)
		draft.Height = productPageMeasure(p["height"], false)
		draft.Depth = productPageMeasure(p["depth"], false)
		if properties, ok := p["additionalProperty"].([]any); ok {
			for _, item := range properties {
				if len(draft.Attributes) >= 64 {
					break
				}
				if property, ok := item.(map[string]any); ok {
					name := productPageString(property["name"], 255)
					value := productPageString(property["value"], 1024)
					if name != "" && value != "" {
						draft.Attributes[name] = value
					}
				}
			}
		}
	}
	if draft.Name == "" {
		draft.Name = productPageString(meta["og:title"], 255)
	}
	if draft.Description == "" {
		draft.Description = productPageString(meta["description"], 16000)
		if draft.Description == "" {
			draft.Description = productPageString(meta["og:description"], 16000)
		}
	}
	if draft.Name == "" {
		return draft, errors.New("No bounded product name found on this page")
	}
	return draft, nil
}
