package api

import (
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
)

// constants
const (
	SUFFIX     = ".svg"
	READ_URL   = "/oo/"
	CREATE_URL = "/svg/"
	STATIC_URL = "/static/"
)

// weak constants
var (
	VALIDATOR = regexp.MustCompile(`(?s)<svg\b[^>]*>(.*?)<\/svg>`)
	ENCODER   = base64.URLEncoding.WithPadding(base64.StdPadding).Strict()
)

var mux = http.NewServeMux()

//go:embed templates
var templates embed.FS

// Link is the main output structure
type Link struct {
	Href string `json:"href"`
}

var (
	Debug *log.Logger
	Info  *log.Logger
	Error *log.Logger
)

const logFlags = log.Ltime

func init() {
	Debug = log.New(os.Stderr, "\033[37m   DEBUG ", logFlags)
	Info = log.New(os.Stdout, "\033[97m    INFO ", logFlags)
	Error = log.New(os.Stderr, "\033[31m   ERROR ", logFlags)

	mux.HandleFunc(READ_URL, read)
	mux.HandleFunc(CREATE_URL, create)
	mux.HandleFunc("/", index)

}

func userError(w http.ResponseWriter, msg string, code int) {
	Error.Println(msg)
	http.Error(w, msg, code)
}

func serverError(w http.ResponseWriter, msg string, code int) {
	Error.Println(msg)
	http.Error(w, "server error", code)
}

// encode compresses an input buffer and encode it into base64
func encode(raw []byte) (string, error) {
	var tmp bytes.Buffer
	w := zlib.NewWriter(&tmp)
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	data := tmp.Bytes()
	return ENCODER.EncodeToString(data), nil
}

// decode decodes the base64 input string and uncompress it
func decode(str string) ([]byte, error) {
	raw, err := ENCODER.DecodeString(str)
	if err != nil {
		return nil, fmt.Errorf("error while decoding string: %v", err)
	}
	tmp := bytes.NewBuffer(raw)
	r, err := zlib.NewReader(tmp)
	if err != nil {
		return nil, fmt.Errorf("error while uncompressing payload: %v", err)
	}

	return io.ReadAll(r)
}

func read(w http.ResponseWriter, r *http.Request) {
	content := strings.TrimPrefix(r.URL.Path, "/oo/")
	content = strings.TrimSuffix(content, ".svg")

	data, err := decode(content)
	if err != nil {
		userError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !VALIDATOR.Match(data) {
		userError(w, fmt.Sprintf("invalid svg: %s", data), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	if _, err := w.Write(data); err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
	}
}

func create(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	// direct https connection or behind a proxy
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	content := r.FormValue("content")
	content = strings.Trim(content, "\r\n")
	if content == "" {
		userError(w, "empty payload", http.StatusBadRequest)
		return
	}

	if !VALIDATOR.MatchString(content) {
		userError(w, fmt.Sprintf("invalid svg: %s", content), http.StatusBadRequest)
		return
	}

	Debug.Printf("content: %s\n", content)
	data, err := encode([]byte(content))

	if err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, err := template.ParseFS(templates, "templates/base.html", "templates/result.html")
	if err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	link := Link{Href: fmt.Sprintf("%s://%s%s%s", scheme, r.Host, READ_URL, data)}

	if r.Header.Get("Accept") == "application/json" {
		bytes, err := json.Marshal(link)
		if err != nil {
			serverError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(bytes); err != nil {
			serverError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := t.Execute(w, link); err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		userError(w, fmt.Sprintf("method not allowed: %v\n", r.Method), http.StatusMethodNotAllowed)
		return
	}
	t, err := template.ParseFS(templates, "templates/base.html", "templates/form.html")
	if err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, Link{Href: CREATE_URL}); err != nil {
		serverError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func loggingMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Info.Printf("%s %s", r.Method, r.URL.Path)
		// Pass control back to the handler
		handler.ServeHTTP(w, r)
	})
}

func Handler(w http.ResponseWriter, r *http.Request) {
	loggingMiddleware(mux).ServeHTTP(w, r)
}
