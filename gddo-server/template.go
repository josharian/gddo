// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	godoc "go/doc"
	htemp "html/template"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	ttemp "text/template"
	"time"

	"github.com/golang/gddo/doc"
	"github.com/golang/gddo/gosrc"
	"github.com/golang/gddo/httputil"
)

var cacheBusters httputil.CacheBusters

type flashMessage struct {
	ID   string
	Args []string
}

// getFlashMessages retrieves flash messages from the request and clears the flash cookie if needed.
func getFlashMessages(resp http.ResponseWriter, req *http.Request) []flashMessage {
	c, err := req.Cookie("flash")
	if err == http.ErrNoCookie {
		return nil
	}
	http.SetCookie(resp, &http.Cookie{Name: "flash", Path: "/", MaxAge: -1, Expires: time.Now().Add(-100 * 24 * time.Hour)})
	if err != nil {
		return nil
	}
	p, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		return nil
	}
	var messages []flashMessage
	for _, s := range strings.Split(string(p), "\000") {
		idArgs := strings.Split(s, "\001")
		messages = append(messages, flashMessage{ID: idArgs[0], Args: idArgs[1:]})
	}
	return messages
}

// setFlashMessages sets a cookie with the given flash messages.
func setFlashMessages(resp http.ResponseWriter, messages []flashMessage) {
	var buf []byte
	for i, message := range messages {
		if i > 0 {
			buf = append(buf, '\000')
		}
		buf = append(buf, message.ID...)
		for _, arg := range message.Args {
			buf = append(buf, '\001')
			buf = append(buf, arg...)
		}
	}
	value := base64.URLEncoding.EncodeToString(buf)
	http.SetCookie(resp, &http.Cookie{Name: "flash", Value: value, Path: "/"})
}

type tdoc struct {
	*doc.Package
	allExamples []*texample
}

type texample struct {
	Id      string
	Label   string
	Example *doc.Example
	obj     interface{}
}

func newTDoc(pdoc *doc.Package) *tdoc {
	return &tdoc{Package: pdoc}
}

func (pdoc *tdoc) SourceLink(pos doc.Pos, text, anchor string) htemp.HTML {
	text = htemp.HTMLEscapeString(text)
	if pos.Line == 0 || pdoc.LineFmt == "" || pdoc.Files[pos.File].URL == "" {
		return htemp.HTML(text)
	}
	var u string
	if anchor != "" && strings.HasPrefix(pdoc.Files[pos.File].URL, "/") {
		u = fmt.Sprintf("%s#%s", pdoc.Files[pos.File].URL, anchor)
	} else {
		u = fmt.Sprintf(pdoc.LineFmt, pdoc.Files[pos.File].URL, pos.Line)
	}
	u = htemp.HTMLEscapeString(u)
	return htemp.HTML(fmt.Sprintf(`<a title="View Source" href="%s">%s</a>`, u, text))
}

func (pdoc *tdoc) PageName() string {
	if pdoc.Name != "" && !pdoc.IsCmd {
		return pdoc.Name
	}
	_, name := path.Split(pdoc.ImportPath)
	return name
}

func (pdoc *tdoc) addExamples(obj interface{}, export, method string, examples []*doc.Example) {
	label := export
	id := export
	if method != "" {
		label += "." + method
		id += "-" + method
	}
	for _, e := range examples {
		te := &texample{Label: label, Id: id, Example: e, obj: obj}
		if e.Name != "" {
			te.Label += " (" + e.Name + ")"
			if method == "" {
				te.Id += "-"
			}
			te.Id += "-" + e.Name
		}
		pdoc.allExamples = append(pdoc.allExamples, te)
	}
}

type byExampleId []*texample

func (e byExampleId) Len() int           { return len(e) }
func (e byExampleId) Less(i, j int) bool { return e[i].Id < e[j].Id }
func (e byExampleId) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }

func (pdoc *tdoc) AllExamples() []*texample {
	if pdoc.allExamples != nil {
		return pdoc.allExamples
	}
	pdoc.allExamples = make([]*texample, 0)
	pdoc.addExamples(pdoc, "package", "", pdoc.Examples)
	for _, f := range pdoc.Funcs {
		pdoc.addExamples(f, f.Name, "", f.Examples)
	}
	for _, t := range pdoc.Types {
		pdoc.addExamples(t, t.Name, "", t.Examples)
		for _, f := range t.Funcs {
			pdoc.addExamples(f, f.Name, "", f.Examples)
		}
		for _, m := range t.Methods {
			if len(m.Examples) > 0 {
				pdoc.addExamples(m, t.Name, m.Name, m.Examples)
			}
		}
	}
	sort.Sort(byExampleId(pdoc.allExamples))
	return pdoc.allExamples
}

func (pdoc *tdoc) ObjExamples(obj interface{}) []*texample {
	var examples []*texample
	for _, e := range pdoc.allExamples {
		if e.obj == obj {
			examples = append(examples, e)
		}
	}
	return examples
}

func (pdoc *tdoc) Breadcrumbs(templateName string) htemp.HTML {
	if !strings.HasPrefix(pdoc.ImportPath, pdoc.ProjectRoot) {
		return ""
	}
	var buf bytes.Buffer
	i := 0
	j := len(pdoc.ProjectRoot)
	if j == 0 {
		j = strings.IndexRune(pdoc.ImportPath, '/')
		if j < 0 {
			j = len(pdoc.ImportPath)
		}
	}
	for {
		if i != 0 {
			buf.WriteString(`<span class="text-muted">/</span>`)
		}
		link := j < len(pdoc.ImportPath) ||
			(templateName != "dir.html" && templateName != "cmd.html" && templateName != "pkg.html")
		if link {
			buf.WriteString(`<a href="`)
			buf.WriteString(formatPathFrag(pdoc.ImportPath[:j], ""))
			buf.WriteString(`">`)
		} else {
			buf.WriteString(`<span class="text-muted">`)
		}
		buf.WriteString(htemp.HTMLEscapeString(pdoc.ImportPath[i:j]))
		if link {
			buf.WriteString("</a>")
		} else {
			buf.WriteString("</span>")
		}
		i = j + 1
		if i >= len(pdoc.ImportPath) {
			break
		}
		j = strings.IndexRune(pdoc.ImportPath[i:], '/')
		if j < 0 {
			j = len(pdoc.ImportPath)
		} else {
			j += i
		}
	}
	return htemp.HTML(buf.String())
}

func formatPathFrag(path, fragment string) string {
	if len(path) > 0 && path[0] != '/' {
		path = "/" + path
	}
	u := url.URL{Path: path, Fragment: fragment}
	return u.String()
}

func hostFn(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return u.Host
}

func mapFn(kvs ...interface{}) (map[string]interface{}, error) {
	if len(kvs)%2 != 0 {
		return nil, errors.New("map requires even number of arguments.")
	}
	m := make(map[string]interface{})
	for i := 0; i < len(kvs); i += 2 {
		s, ok := kvs[i].(string)
		if !ok {
			return nil, errors.New("even args to map must be strings.")
		}
		m[s] = kvs[i+1]
	}
	return m, nil
}

// relativePathFn formats an import path as HTML.
func relativePathFn(path string, parentPath interface{}) string {
	if p, ok := parentPath.(string); ok && p != "" && strings.HasPrefix(path, p) {
		path = path[len(p)+1:]
	}
	return path
}

// importPathFn formats an import with zero width space characters to allow for breaks.
func importPathFn(path string) htemp.HTML {
	path = htemp.HTMLEscapeString(path)
	if len(path) > 45 {
		// Allow long import paths to break following "/"
		path = strings.Replace(path, "/", "/&#8203;", -1)
	}
	return htemp.HTML(path)
}

var (
	h3Pat      = regexp.MustCompile(`<h3 id="([^"]+)">([^<]+)</h3>`)
	rfcPat     = regexp.MustCompile(`RFC\s+(\d{3,4})`)
	packagePat = regexp.MustCompile(`\s+package\s+([-a-z0-9]\S+)`)
)

func replaceAll(src []byte, re *regexp.Regexp, replace func(out, src []byte, m []int) []byte) []byte {
	var out []byte
	for len(src) > 0 {
		m := re.FindSubmatchIndex(src)
		if m == nil {
			break
		}
		out = append(out, src[:m[0]]...)
		out = replace(out, src, m)
		src = src[m[1]:]
	}
	if out == nil {
		return src
	}
	return append(out, src...)
}

// commentFn formats a source code comment as HTML.
func commentFn(v string) htemp.HTML {
	var buf bytes.Buffer
	godoc.ToHTML(&buf, v, nil)
	p := buf.Bytes()
	p = replaceAll(p, h3Pat, func(out, src []byte, m []int) []byte {
		out = append(out, `<h4 id="`...)
		out = append(out, src[m[2]:m[3]]...)
		out = append(out, `">`...)
		out = append(out, src[m[4]:m[5]]...)
		out = append(out, ` <a class="permalink" href="#`...)
		out = append(out, src[m[2]:m[3]]...)
		out = append(out, `">&para</a></h4>`...)
		return out
	})
	p = replaceAll(p, rfcPat, func(out, src []byte, m []int) []byte {
		out = append(out, `<a href="http://tools.ietf.org/html/rfc`...)
		out = append(out, src[m[2]:m[3]]...)
		out = append(out, `">`...)
		out = append(out, src[m[0]:m[1]]...)
		out = append(out, `</a>`...)
		return out
	})
	p = replaceAll(p, packagePat, func(out, src []byte, m []int) []byte {
		path := bytes.TrimRight(src[m[2]:m[3]], ".!?:")
		if !gosrc.IsValidPath(string(path)) {
			return append(out, src[m[0]:m[1]]...)
		}
		out = append(out, src[m[0]:m[2]]...)
		out = append(out, `<a href="/`...)
		out = append(out, path...)
		out = append(out, `">`...)
		out = append(out, path...)
		out = append(out, `</a>`...)
		out = append(out, src[m[2]+len(path):m[1]]...)
		return out
	})
	return htemp.HTML(p)
}

// commentTextFn formats a source code comment as text.
func commentTextFn(v string) string {
	const indent = "    "
	var buf bytes.Buffer
	godoc.ToText(&buf, v, indent, "\t", 80-2*len(indent))
	p := buf.Bytes()
	return string(p)
}

var period = []byte{'.'}

func codeFn(c doc.Code, typ *doc.Type) htemp.HTML {
	var buf bytes.Buffer
	last := 0
	src := []byte(c.Text)
	for _, a := range c.Annotations {
		htemp.HTMLEscape(&buf, src[last:a.Pos])
		switch a.Kind {
		case doc.PackageLinkAnnotation:
			buf.WriteString(`<a href="`)
			buf.WriteString(formatPathFrag(c.Paths[a.PathIndex], ""))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.LinkAnnotation, doc.BuiltinAnnotation:
			var p string
			if a.Kind == doc.BuiltinAnnotation {
				p = "builtin"
			} else if a.PathIndex >= 0 {
				p = c.Paths[a.PathIndex]
			}
			n := src[a.Pos:a.End]
			n = n[bytes.LastIndex(n, period)+1:]
			buf.WriteString(`<a href="`)
			buf.WriteString(formatPathFrag(p, string(n)))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.CommentAnnotation:
			buf.WriteString(`<span class="com">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		case doc.AnchorAnnotation:
			buf.WriteString(`<span id="`)
			if typ != nil {
				htemp.HTMLEscape(&buf, []byte(typ.Name))
				buf.WriteByte('.')
			}
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		default:
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
		}
		last = int(a.End)
	}
	htemp.HTMLEscape(&buf, src[last:])
	return htemp.HTML(buf.String())
}

var gaAccount string

func gaAccountFn() string {
	return gaAccount
}

func noteTitleFn(s string) string {
	return strings.Title(strings.ToLower(s))
}

func htmlCommentFn(s string) htemp.HTML {
	return htemp.HTML("<!-- " + s + " -->")
}

var mimeTypes = map[string]string{
	".html": htmlMIMEType,
	".txt":  textMIMEType,
}

func executeTemplate(resp http.ResponseWriter, name string, status int, header http.Header, data interface{}) error {
	for k, v := range header {
		resp.Header()[k] = v
	}
	mimeType, ok := mimeTypes[path.Ext(name)]
	if !ok {
		mimeType = textMIMEType
	}
	resp.Header().Set("Content-Type", mimeType)
	t := templates[name]
	if t == nil {
		return fmt.Errorf("Template %s not found", name)
	}
	resp.WriteHeader(status)
	if status == http.StatusNotModified {
		return nil
	}
	return t.Execute(resp, data)
}

var templates = map[string]interface {
	Execute(io.Writer, interface{}) error
}{}

func joinTemplateDir(base string, files []string) []string {
	result := make([]string, len(files))
	for i := range files {
		result[i] = filepath.Join(base, "templates", files[i])
	}
	return result
}

func parseHTMLTemplates(sets [][]string) error {
	for _, set := range sets {
		templateName := set[0]
		t := htemp.New("")
		t.Funcs(htemp.FuncMap{
			"code":              codeFn,
			"comment":           commentFn,
			"equal":             reflect.DeepEqual,
			"gaAccount":         gaAccountFn,
			"host":              hostFn,
			"htmlComment":       htmlCommentFn,
			"importPath":        importPathFn,
			"isValidImportPath": gosrc.IsValidPath,
			"map":               mapFn,
			"noteTitle":         noteTitleFn,
			"relativePath":      relativePathFn,
			"sidebarEnabled":    func() bool { return *sidebarEnabled },
			"staticPath":        func(p string) string { return cacheBusters.AppendQueryParam(p, "v") },
			"templateName":      func() string { return templateName },
		})
		if _, err := t.ParseFiles(joinTemplateDir(*assetsDir, set)...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", set)
		}
		templates[set[0]] = t
	}
	return nil
}

func parseTextTemplates(sets [][]string) error {
	for _, set := range sets {
		t := ttemp.New("")
		t.Funcs(ttemp.FuncMap{
			"comment": commentTextFn,
		})
		if _, err := t.ParseFiles(joinTemplateDir(*assetsDir, set)...); err != nil {
			return err
		}
		t = t.Lookup("ROOT")
		if t == nil {
			return fmt.Errorf("ROOT template not found in %v", set)
		}
		templates[set[0]] = t
	}
	return nil
}
