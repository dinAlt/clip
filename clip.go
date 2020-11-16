package clip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"reflect"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

// Params are used to tweak Convert output.
type Params struct {
	Query          *string `json:"query,omitempty"`           // css selector to be included in resulted PDF document
	RemoveQuery    *string `json:"remove_query,omitempty"`    // css selector of elements to be removed
	CustomStyles   *string `json:"custom_styles,omitempty"`   // custom css styles to be injected into doc
	WithContainers *bool   `json:"with_containers,omitempty"` // preserve all containert from document body to selector query result
	// global options
	Grayscale    *bool   `json:"grayscale,omitempty"`
	MarginBottom *uint   `json:"margin_bottom,omitempty"`
	MarginLeft   *uint   `json:"margin_left,omitempty"`
	MarginRight  *uint   `json:"margin_right,omitempty"`
	MarginTop    *uint   `json:"margin_top,omitempty"`
	Orientation  *string `json:"orientation,omitempty"`
	PageHeight   *uint   `json:"page_height,omitempty"`
	PageWidth    *uint   `json:"page_width,omitempty"`
	PageSize     *string `json:"page_size,omitempty"`
	Title        *string `json:"title,omitempty"`
	// page options
	DisableExternalLinks *bool    `json:"disable_external_links,omitempty"`
	DisableInternalLinks *bool    `json:"disable_internal_links,omitempty"`
	DisableJavascript    *bool    `json:"disable_javascript,omitempty"`
	NoBackground         *bool    `json:"no_background,omitempty"`
	NoImages             *bool    `json:"no_images,omitempty"`
	PageOffset           *uint    `json:"page_offset,omitempty"`
	Zoom                 *float64 `json:"zoom,omitempty"`
	ViewportSize         *string  `json:"viewport_size,omitempty"`
}

func (p *Params) String() string {
	if p == nil {
		return "clip.Params(nil)"
	}
	pt := reflect.TypeOf(p).Elem()
	pv := reflect.ValueOf(p).Elem()
	fcnt := pt.NumField()
	sb := strings.Builder{}
	sb.WriteString("clip.Params{ ")
	for i := 0; i < fcnt; i++ {
		fld := pv.Field(i)
		if fld.IsNil() {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %v; ", pt.Field(i).Name,
			fld.Elem().Interface()))
	}
	sb.WriteString("}")
	return sb.String()
}

func (p *Params) validate() error {
	err := checkSize(p.PageSize)
	if err != nil {
		return &ValidationError{err.Error()}
	}
	if p.Orientation != nil &&
		*p.Orientation != wkhtmltopdf.OrientationLandscape &&
		*p.Orientation != wkhtmltopdf.OrientationPortrait {
		return &ValidationError{"bad value for orientation parameter: " +
			*p.Orientation}
	}
	return nil
}

func (p *Params) skipDOMProcess() bool {
	return p.Query == nil && p.RemoveQuery == nil &&
		p.CustomStyles == nil && p.DisableJavascript == nil
}

func (p *Params) merge(g *wkhtmltopdf.PDFGenerator) {
	if p.Grayscale != nil {
		g.Grayscale.Set(*p.Grayscale)
	}
	if p.MarginBottom != nil {
		g.MarginBottom.Set(*p.MarginBottom)
	}
	if p.MarginLeft != nil {
		g.MarginLeft.Set(*p.MarginLeft)
	}
	if p.MarginRight != nil {
		g.MarginRight.Set(*p.MarginRight)
	}
	if p.MarginTop != nil {
		g.MarginTop.Set(*p.MarginTop)
	}
	if p.Orientation != nil {
		g.Orientation.Set(*p.Orientation)
	}
	if p.PageHeight != nil {
		g.PageHeight.Set(*p.PageHeight)
	}
	if p.PageWidth != nil {
		g.PageWidth.Set(*p.PageWidth)
	}
	if p.PageSize != nil {
		g.PageSize.Set(*p.PageSize)
	}
	if p.Title != nil {
		g.Title.Set(*p.Title)
	}

	if p.DisableExternalLinks != nil {
		g.Cover.DisableExternalLinks.Set(*p.DisableExternalLinks)
		g.TOC.DisableExternalLinks.Set(*p.DisableExternalLinks)
	}
	if p.DisableInternalLinks != nil {
		g.Cover.DisableInternalLinks.Set(*p.DisableInternalLinks)
		g.TOC.DisableInternalLinks.Set(*p.DisableInternalLinks)
	}
	if p.DisableJavascript != nil {
		g.Cover.DisableJavascript.Set(*p.DisableJavascript)
		g.TOC.DisableJavascript.Set(*p.DisableJavascript)
		g.Cover.JavascriptDelay.Set(1)
		g.TOC.JavascriptDelay.Set(1)
	}
	if p.NoBackground != nil {
		g.Cover.NoBackground.Set(*p.NoBackground)
		g.TOC.NoBackground.Set(*p.NoBackground)
	}
	if p.NoImages != nil {
		g.Cover.NoImages.Set(*p.NoImages)
		g.TOC.NoImages.Set(*p.NoImages)
	}
	if p.PageOffset != nil {
		g.Cover.PageOffset.Set(*p.PageOffset)
		g.TOC.PageOffset.Set(*p.PageOffset)
	}
	if p.Zoom != nil {
		g.Cover.Zoom.Set(*p.Zoom)
		g.TOC.Zoom.Set(*p.Zoom)
	}

	g.Cover.LoadErrorHandling.Set("ignore")
	g.TOC.LoadErrorHandling.Set("ignore")
}

// Package errors.
var (
	ErrBadStatus       = errors.New("bad status")
	ErrNoQueryResult   = errors.New("no result")
	ErrBadURLScheme    = errors.New("bad URL scheme")
	ErrInvalidPageSize = errors.New("invalid page size")
)

// IgnoredError returned just for logging.
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

type ValidationError struct {
	Message string
}

// Error is error interface implementation.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s", e.Message)
}

func Convert(url string, w io.Writer, p *Params) error {
	return ConvertCtx(context.Background(), url, w, p)
}

// ConvertCtx downloads page from url, converts it to PDF via wkhtmltopdf
// and writes result to w.
func ConvertCtx(ctx context.Context, url string, w io.Writer, p *Params) error {
	if ctx == nil {
		panic("clip.ConvertCtx: ctx is nil")
	}
	if w == nil {
		panic("clip.ConvertCtx: w is nil")
	}
	if p == nil {
		panic("clip.ConvertCtx: params is nil")
	}
	err := p.validate()
	if err != nil {
		return err
	}

	gen, err := wkhtmltopdf.NewPDFGenerator()
	if err != nil {
		return fmt.Errorf("wkhtmltopdf.NewPDFGenerator: %w", err)
	}
	tURL, err := neturl.Parse(url)
	if err != nil {
		return &URLError{err}
	}
	if tURL.Scheme != "http" && tURL.Scheme != "https" { // ensure user not trying to get file from our local disk
		return fmt.Errorf("%w: %s", ErrBadURLScheme, tURL.Scheme)
	}
	p.merge(gen)

	switch {
	case p.skipDOMProcess():
		gen.AddPage(wkhtmltopdf.NewPage(url))
	default:
		txt, err := getHTML(ctx, tURL, p)
		if err != nil {
			return err
		}
		r := strings.NewReader(txt)
		pr := wkhtmltopdf.NewPageReader(r)
		gen.AddPage(pr)
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
func getHTML(ctx context.Context, url *neturl.URL, p *Params) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
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
	doc.Url = url

	applyChanges(doc, p)
	if len(doc.Find("body").Children().Nodes) == 0 {
		return "", ErrNoQueryResult
	}

	txt, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("doc.Html: %w", err)
	}

	return txt, nil
}

// applyChanges removes elements not matching css queries in qs from DOM body.
// It preserves containers structure if preserveContainers == true.
func applyChanges(doc *goquery.Document, p *Params) {
	body := doc.Find("body")
	if p.Query != nil && len(*p.Query) > 0 {
		sel := doc.Find(*p.Query)
		if p.WithContainers != nil && *p.WithContainers {
			sel = containerize(sel, body)
		}
		body.Children().Remove()
		body.AppendSelection(sel)
	}
	if p.RemoveQuery != nil && len(*p.RemoveQuery) > 0 {
		doc.Find(*p.RemoveQuery).Remove()
	}
	if p.CustomStyles != nil && len(*p.CustomStyles) > 0 {
		doc.Find("head").AppendHtml("<style>" + *p.CustomStyles + "</style>")
	}
	if p.DisableJavascript != nil && *p.DisableJavascript {
		doc.Find("script,link[rel=\"script\"]").Remove()
	}
	convertURLs(doc)
}

// convertURLs makes URLs absolute
func convertURLs(doc *goquery.Document) {
	for _, n := range doc.Find("[href],[src]").Nodes {
		for i := range n.Attr {
			if n.Attr[i].Key == "href" || n.Attr[i].Key == "src" {
				url, err := neturl.Parse(n.Attr[i].Val)
				if err != nil || url.IsAbs() {
					continue
				}
				if url.Host == "" {
					url.Host = doc.Url.Host
					url.Scheme = doc.Url.Scheme
				}
				switch {
				case url.Scheme == "":
					url.Scheme = doc.Url.Scheme
				case url.Path != "" && !strings.HasPrefix(url.Path, "/"):
					url.Path = doc.Url.Path + "/" + url.Path
				case url.Fragment != "":
					url.Path = doc.Url.Path
				}
				n.Attr[i].Val = url.String()
			}
		}
	}
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

func checkSize(size *string) error {
	if size == nil {
		return nil
	}
	switch *size {
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
	return fmt.Errorf("%w: %s", ErrInvalidPageSize, *size)
}
