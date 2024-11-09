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

const BANNER = "\033[33m" + `
███████╗██╗   ██╗ ██████╗  ██████╗ 
██╔════╝██║   ██║██╔════╝ ██╔════╝ 
███████╗██║   ██║██║  ███╗██║  ███╗
╚════██║╚██╗ ██╔╝██║   ██║██║   ██║
███████║ ╚████╔╝ ╚██████╔╝╚██████╔╝
╚══════╝  ╚═══╝   ╚═════╝  ╚═════╝ 
` + "\033[0m"

// constants
const (
	SUFFIX     = ".svg"
	READ_URL   = "/oo/"
	CREATE_URL = "/svg/"
	STATIC_URL = "/static/"
)

// weak constants
var (
	SANITIZER = regexp.MustCompile(`^[a-zA-Z0-9<>!"'./%= :\-,\n\r?_#]+$`)
	ENCODER   = base64.URLEncoding.WithPadding(base64.StdPadding).Strict()
)

// config
var (
	host      = "127.0.0.1"
	port uint = 4444
)

var mux = http.NewServeMux()

//go:embed static
// var static embed.FS

//go:embed templates
var templates embed.FS

// Link is the main output structure
type Link struct {
	Href string `json:"href"`
}

var (
	Debug    *log.Logger
	Info     *log.Logger
	Warning  *log.Logger
	Error    *log.Logger
	Critical *log.Logger
)

const logFlags = log.Ltime

func init() {
	Debug = log.New(os.Stderr, "\033[37m   DEBUG ", logFlags)
	Info = log.New(os.Stdout, "\033[97m    INFO ", logFlags)
	Warning = log.New(os.Stdout, "\033[33m WARNING ", logFlags)
	Error = log.New(os.Stderr, "\033[31m   ERROR ", logFlags)
	Critical = log.New(os.Stderr, "\033[41mCRITICAL ", logFlags)

	// staticFS := http.FileServer(http.FS(static))
	mux.HandleFunc(READ_URL, read)
	mux.HandleFunc(CREATE_URL, create)
	// mux.Handle(STATIC_URL, staticFS)
	mux.HandleFunc("/", index)
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
		Error.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// sanitizedData := SANITIZER.ReplaceAll(data, []byte(""))
	sanitizedData := data
	// if !SANITIZER.Match(data) {
	// 	fmt.Println("error: invalid payload:", string(data))
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	return
	// }
	w.Header().Set("Content-Type", "image/svg+xml")
	if _, err := w.Write(sanitizedData); err != nil {
		Critical.Println(err)
	}
}

func create(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		Warning.Println("empty payload")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	Debug.Printf("content: %s\n", content)
	data, err := encode([]byte(content))

	if err != nil {
		Error.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := template.ParseFS(templates, "templates/base.html", "templates/result.html")
	if err != nil {
		Error.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	link := Link{Href: fmt.Sprintf("%s://%s%s%s", scheme, r.Host, READ_URL, data)}

	if r.Header.Get("Accept") == "application/json" {
		bytes, err := json.Marshal(link)
		if err != nil {
			Critical.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(bytes); err != nil {
			Critical.Println(err)
		}
		return
	}

	if err := t.Execute(w, link); err != nil {
		Critical.Println(err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		Warning.Printf("method not allowed: %v\n", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	t, err := template.ParseFS(templates, "templates/base.html", "templates/form.html")
	if err != nil {
		Critical.Panicln(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, Link{Href: CREATE_URL}); err != nil {
		Error.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
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
