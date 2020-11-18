package clip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

var (
	PrintArgs           bool   // print wkhtmltopdf args
	SaveProcessedHTMLTo string // save processed HTML to directory (used if not empty)
)

func init() {
	if os.Getenv("CLIP_PRINT_ARGS") != "" {
		PrintArgs = true
	}
	SaveProcessedHTMLTo = os.Getenv("CLIP_SAVE_HTML")
}

// Params are used to tweak ToPDF output.
type Params struct {
	Query             *string `json:"query,omitempty"`               // css selector to be included in resulted PDF document
	Remove            *string `json:"remove,omitempty"`              // css selector of elements to be removed
	NoBreakBefore     *string `json:"no_break_before,omitempty"`     // css selector for elements to set break-before:avoid-page
	NoBreakInside     *string `json:"no_break_inside,omitempty"`     // css selector for elements to set break-inside:avoid-page
	NoBreakAfter      *string `json:"no_break_after,omitempty"`      // css selector for elements to set break-after:avoid-page
	CustomStyles      *string `json:"custom_styles,omitempty"`       // custom css styles to be injected into doc
	WithContainers    *bool   `json:"with_containers,omitempty"`     // preserve all containert from document body to selector query result
	ForceImageLoading *bool   `json:"force_image_loading,omitempty"` // replace img[src] by img[data-src] conetnt
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
	EnableJavascript     *bool    `json:"enable_javascript,omitempty"`
	NoBackground         *bool    `json:"no_background,omitempty"`
	NoImages             *bool    `json:"no_images,omitempty"`
	PageOffset           *uint    `json:"page_offset,omitempty"`
	Zoom                 *float64 `json:"zoom,omitempty"`
	ViewportSize         *string  `json:"viewport_size,omitempty"`
}

// AddFrom adds missed in p values from o.
// Don't overwrites existed values.
func (p *Params) AddFrom(o *Params) {
	if p == nil {
		panic("clip.Params.AddFrom: p is nil")
	}
	if p == nil {
		panic("clip.Params.AddFrom: o is nil")
	}
	vp := reflect.ValueOf(p).Elem()
	vo := reflect.ValueOf(o).Elem()
	fcnt := vp.NumField()
	for i := 0; i < fcnt; i++ {
		fp := vp.Field(i)
		if !fp.IsNil() {
			continue
		}
		fo := vo.Field(i)
		if fo.IsNil() {
			continue
		}
		fo = fo.Elem()
		nv := reflect.New(fo.Type())
		nv.Elem().Set(fo)
		fp.Set(nv)
	}
}

// String pretty prints struct values
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
	return p.Query == nil && p.Remove == nil &&
		p.CustomStyles == nil && p.ForceImageLoading == nil &&
		p.NoBreakBefore != nil && p.NoBreakInside == nil &&
		p.NoBreakAfter != nil
}

func (p *Params) mergeGen(g *wkhtmltopdf.PDFGenerator) {
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
}

func (p *Params) mergePageOptions(o *wkhtmltopdf.PageOptions) { //nolint:unused
	if p.DisableExternalLinks != nil {
		o.DisableExternalLinks.Set(*p.DisableExternalLinks)
	}
	if p.DisableInternalLinks != nil {
		o.DisableInternalLinks.Set(*p.DisableInternalLinks)
	}
	o.DisableJavascript.Set(p.EnableJavascript == nil || !*p.EnableJavascript)
	if p.NoBackground != nil {
		o.NoBackground.Set(*p.NoBackground)
	}
	if p.NoImages != nil {
		o.NoImages.Set(*p.NoImages)
	}
	if p.PageOffset != nil {
		o.PageOffset.Set(*p.PageOffset)
	}
	if p.Zoom != nil {
		o.Zoom.Set(*p.Zoom)
	}
}

// Package errors.
var (
	ErrBadStatus       = errors.New("bad status")
	ErrNoQueryResult   = errors.New("no result")
	ErrNoURL           = errors.New("url is required")
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

func ToPDF(url string, w io.Writer, p *Params) error {
	return ToPDFCtx(context.Background(), url, w, p)
}

// ToPDFCtx downloads page from url, converts it to PDF via wkhtmltopdf
// and writes result to w.
func ToPDFCtx(ctx context.Context, url string, w io.Writer, p *Params) error {
	if ctx == nil {
		panic("clip.ToPDFCtx: ctx is nil")
	}
	if w == nil {
		panic("clip.ToPDFCtx: w is nil")
	}
	if p == nil {
		panic("clip.ToPDFCtx: params is nil")
	}
	err := p.validate()
	if err != nil {
		return err
	}
	if url == "" {
		return ErrNoURL
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

	switch {
	case p.skipDOMProcess():
		pg := wkhtmltopdf.NewPage(url)
		p.mergePageOptions(&pg.PageOptions)
		gen.AddPage(pg)
	default:
		txt, err := getHTML(ctx, tURL, p)
		if err != nil {
			return err
		}
		r := strings.NewReader(txt)
		pr := wkhtmltopdf.NewPageReader(r)
		p.mergePageOptions(&pr.PageOptions)
		gen.AddPage(pr)
	}
	p.mergeGen(gen)
	if PrintArgs {
		fmt.Fprintln(os.Stderr, "wkhtmltopdf args:", gen.ArgString())
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

// getHTML returns processed with Params from p html string
func getHTML(ctx context.Context, url *neturl.URL, p *Params) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		return "", fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.Header.Set("user-agent", "clip-to-pdf/1.0")
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

	err = dump(url, txt)
	if err != nil {
		return "", err
	}

	return txt, nil
}

// dump html to <SaveProcessedHTMLTo>/<domain name> folder
// if SaveProcessedHTMLTo != "". Name of file will be set to
// last segment of path with .html extension, or index.html,
// if path is empty.
func dump(url *neturl.URL, html string) error {
	if SaveProcessedHTMLTo == "" {
		return nil
	}
	dir := filepath.Join(SaveProcessedHTMLTo, url.Host)
	err := os.MkdirAll(dir, 0755) //nolint:gosec
	if err != nil {
		return fmt.Errorf("os.MkdirAll: %w", err)
	}
	_, fn := path.Split(url.Path)

	switch {
	case fn == "":
		fn = "index.html"
	case strings.ToLower(path.Ext(fn)) != ".html":
		fn += ".html"
	}

	err = ioutil.WriteFile(filepath.Join(dir, fn), []byte(html), 0644) //nolint:gosec
	if err != nil {
		return fmt.Errorf("os.WriteFile: %w", err)
	}
	return nil
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
	if p.Remove != nil && len(*p.Remove) > 0 {
		doc.Find(*p.Remove).Remove()
	}
	if p.ForceImageLoading != nil && *p.ForceImageLoading {
		doc.Find("img").Each(func(_ int, sel *goquery.Selection) {
			v, _ := sel.Attr("data-src")
			if v == "" {
				return
			}
			sel.SetAttr("src", v)
		})
	}
	head := doc.Find("head")
	if p.NoBreakBefore != nil && len(*p.NoBreakBefore) > 0 {
		head.AppendHtml("<style>" + *p.NoBreakBefore +
			"{page-break-before:avoid!important;break-before:avoid-page!important}</style>")
	}
	if p.NoBreakInside != nil && len(*p.NoBreakInside) > 0 {
		head.AppendHtml("<style>" + *p.NoBreakInside +
			"{page-break-inside:avoid!important;break-inside:avoid-page!important}</style>")
	}
	if p.NoBreakAfter != nil && len(*p.NoBreakAfter) > 0 {
		head.AppendHtml("<style>" + *p.NoBreakAfter +
			"{page-break-after:avoid!important;break-after:avoid-page!important}</style>")
	}
	if p.CustomStyles != nil && len(*p.CustomStyles) > 0 {
		head.AppendHtml("<style>" + *p.CustomStyles + "</style>")
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
