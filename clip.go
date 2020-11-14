package clip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

// Package errors.
var (
	ErrBadStatus       = errors.New("bad status")
	ErrNoQueryResult   = errors.New("no result")
	ErrBadURLScheme    = errors.New("bad URL scheme")
	ErrInvalidPageSize = errors.New("invalid page size")
)

// IgnoredError returned just for logging purposes.
type IgnoredError struct {
	inner error
}

// Error is error interface implementation.
func (e *IgnoredError) Error() string {
	return e.inner.Error()
}

// URLError wraps error from url.Parse method.
type URLError struct {
	inner error
}

// Error is error interface implementation.
func (e *URLError) Error() string {
	return e.inner.Error()
}

// Params are used to tweak Convert output.
type Params struct {
	Query              string // css selector to be included in resulted PDF document
	PreserveContainers bool   // preserve all containert from document body to selector query result
	Size               string // wkhtmltopdf size param
}

// Convert download page from url, converts it to PDF via wkhtmltopdf
// and writes result to w.
func Convert(ctx context.Context, url string, w io.Writer, p Params) error {
	gen, err := wkhtmltopdf.NewPDFGenerator()
	if err != nil {
		return fmt.Errorf("wkhtmltopdf.NewPDFGenerator: %w", err)
	}
	switch {
	case len(p.Query) == 0:
		tURL, err := neturl.Parse(url)
		if err != nil {
			return &URLError{err}
		}
		if tURL.Scheme != "http" && tURL.Scheme != "https" { // ensure user not trying to get file from our local disk
			return fmt.Errorf("%w: %s", ErrBadURLScheme, tURL.Scheme)
		}
		gen.AddPage(wkhtmltopdf.NewPage(url))
	default:
		txt, err := getHTML(ctx, url, p)
		if err != nil {
			return err
		}
		r := strings.NewReader(txt)
		gen.AddPage(wkhtmltopdf.NewPageReader(r))
	}

	if p.Size != "" {
		err := checkSize(p.Size)
		if err != nil {
			return err
		}
		gen.PageSize.Set(p.Size)
	}

	genErr := gen.Create() // this almost always return some error (underlied process stderr output)
	select {
	case <-ctx.Done():
		return fmt.Errorf("context error: %w", ctx.Err())
	default:
	}

	n, err := io.Copy(w, gen.Buffer())
	if err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}
	if n < 1 {
		if genErr != nil {
			return fmt.Errorf("no PDF was generated: %w", genErr) // now we treat wkhtmltopdf error as unrecoverable
		}
		return fmt.Errorf("no PDF was generated")
	}

	if genErr != nil {
		return &IgnoredError{genErr} // just for logging
	}

	return nil
}

// getHTML returns html string filtered by p.Selectors.
func getHTML(ctx context.Context, url string, p Params) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http.DefaultClient.Do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/200 != 1 {
		return "", fmt.Errorf("%w: %d", ErrBadStatus, resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("goquery.NewDocumentFromReader: %w", err)
	}

	prepare(doc, p.Query, p.PreserveContainers)
	if len(doc.Find("body").Children().Nodes) == 0 {
		return "", ErrNoQueryResult
	}
	txt, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("doc.Html: %w", err)
	}
	return txt, nil
}

// prepare removes elements not matching css queries in qs from DOM body.
// It preserves containers structure if preserveContainers == true.
func prepare(doc *goquery.Document, q string, withContainers bool) {
	body := doc.Find("body")
	sel := doc.Find(q)
	if withContainers {
		sel = containerize(sel, body)
	}
	body.Children().Remove()
	body.AppendSelection(sel)
}

// containerize preserves containers structure from root to sel
func containerize(sel *goquery.Selection, root *goquery.Selection) *goquery.Selection {
	var rounds int
	for len(sel.Parent().Nodes) == 0 || sel.Parent().Nodes[0] != root.Nodes[0] {
		parent := sel.Parent()
		parent.Children().Remove()
		sel = parent.AppendSelection(sel)
		rounds++
		if rounds > 1000 {
			panic("clip.containerize: too many iterations")
		}
	}
	return sel
}

func checkSize(size string) error {
	switch size {
	case wkhtmltopdf.PageSizeA0,
		wkhtmltopdf.PageSizeA1,
		wkhtmltopdf.PageSizeA2,
		wkhtmltopdf.PageSizeA3,
		wkhtmltopdf.PageSizeA4,
		wkhtmltopdf.PageSizeA5,
		wkhtmltopdf.PageSizeA6,
		wkhtmltopdf.PageSizeA7,
		wkhtmltopdf.PageSizeA8,
		wkhtmltopdf.PageSizeA9,
		wkhtmltopdf.PageSizeB0,
		wkhtmltopdf.PageSizeB1,
		wkhtmltopdf.PageSizeB10,
		wkhtmltopdf.PageSizeB2,
		wkhtmltopdf.PageSizeB3,
		wkhtmltopdf.PageSizeB4,
		wkhtmltopdf.PageSizeB5,
		wkhtmltopdf.PageSizeB6,
		wkhtmltopdf.PageSizeB7,
		wkhtmltopdf.PageSizeB8,
		wkhtmltopdf.PageSizeB9,
		wkhtmltopdf.PageSizeC5E,
		wkhtmltopdf.PageSizeComm10E,
		wkhtmltopdf.PageSizeCustom,
		wkhtmltopdf.PageSizeDLE,
		wkhtmltopdf.PageSizeExecutive,
		wkhtmltopdf.PageSizeFolio,
		wkhtmltopdf.PageSizeLedger,
		wkhtmltopdf.PageSizeLegal,
		wkhtmltopdf.PageSizeLetter,
		wkhtmltopdf.PageSizeTabloid:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidPageSize, size)
}
