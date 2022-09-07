/*
This is just a simple 302 redirect gated behind an HTTP POST form.

Files in ./embedded/* are embedded into the binary and templated.

Were this a typical OSS project, I wouldn't have commented a single
line in this program. It is entirely straightforward and easy enough
to understand, excluding the imported chi package.

However, I am going to comment this file to death as a learning
experience for others who care.
*/
package main

// You should never have to *add* or *remove* imports to this section, although
// there may be times where you need to append or change a version prefix at the
// end.
//
// Use Gofumports or similar editor tooling to handle this for you. You should be
// able to type "fmt.Println()" somewhere in this file, save, and the "fmt" package
// should be added automatically by your editor('s tooling).
//
// Take the time to set this up, otherwise you'll die of exhaustion.
import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// These are global variables. In a small program like this, this isn't a problem.
// In a larger program, these variables would be scoped and be added using whatever
// tooling that project uses (flags, config files, web api poll, etc.).
var (
	// The go:embed directive below specifies that the files in the
	// ./embedded folder are to be embedded into the binary.

	//go:embed embedded
	romFS embed.FS

	// Some programs use config files, others flags. As this information is not
	// security-sensitive (https://blog.forcesunseen.com/stop-storing-secrets-in-environment-variables),
	// we're fine to store it in envvars.
	indexURL = os.Getenv("INDEX_URL")
	redirURL = os.Getenv("REDIR_URL")

	// Cooldown ip attempts. This map keeps track of those cooldowns.
	cooldown = make(map[string]time.Time)

	// Finals control final question page availibility. Each only gets one load.
	finals = make(map[string]bool)
)

const (
	romIndex  = "embedded/index.html"
	romRobots = "embedded/robots.txt"
	romQ2     = "embedded/q2.html"
	romQ3     = "embedded/q3.html"

	// Oh my! A secret value in source code! Yes, but context! In reality, this is
	// just an elaborate obfuscation. md5 would have been fine here.
	q1Hash = "$2a$10$RcmgQ593JW.4ZHgtJ8adXeFfrq9BJoiXlRmsmmrAxZSGF4VJXTuXy"
	q2Hash = "$2a$10$1IWVW4buxGjWoUE7qdXNAOU/6mlChkPjvGnoP1addD0SDHRTU7BeK"
	q3Hash = "$2a$10$nkGgZoyIAEPdtNrxGqcxj.oEqAS7sqGIO.8v19IpD9gebVanwGLL2"
)

func main() {
	// If you'd like to learn more about chi, read the package docs.
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	// Chi maps HTTP methods to functions that we can define.
	// These functions must implement the interface http.HandlerFunc.
	r.Get("/robots.txt", robotsTxt(romRobots))

	// Index / Q1
	r.Get("/", rootHandler(romIndex))
	r.Get("/index.html", rootHandler(romIndex))
	r.Post("/submit", submitHandler("/q2.html", []byte(q1Hash)))

	// Q2
	r.Get("/q2.html", q2Handler(romQ2))
	r.Post("/q2", q2SubmitHandler([]byte(q2Hash)))

	// Q3
	r.Route("/final", func(r chi.Router) {
		r.Get("/{guid}.html", q3Handler(romQ3))
		r.Post("/{guid}", q3SubmitHandler(redirURL, []byte(q3Hash)))
	})

	if err := http.ListenAndServe(":8888", r); err != nil {
		log.Fatal(err)
	}
}

func rootHandler(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// template requires no path separators, otherwise it fails at runtime.
		f := filepath.Base(file)
		t, err := template.New(f).Funcs(template.FuncMap{
			// HTML comments are automatically escaped and removed from the output.
			// They must be marked to be kept.
			"noencode": func(s string) template.HTML { return template.HTML(s) },
		}).ParseFS(romFS, file)
		if err != nil {
			// panics are okay in this context because we're using chi middleware
			// to handle recoveries.
			panic("internal error: could not template " + file)
		}
		err = t.ExecuteTemplate(w, f, struct{ IndexURL string }{IndexURL: indexURL})
		if err != nil {
			panic(err)
		}
	}
}

func robotsTxt(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// read the file from the embedded filesystem
		p, _ := romFS.ReadFile(file)
		// and write to the http.ResponseWriter, which implements io.Writer
		_, _ = w.Write(p)
	}
}

func normalizeSubmission(s string) string {
	re := regexp.MustCompile("[^a-z\\s]+")
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = re.ReplaceAllString(s, "")
	return s
}

func submitHandler(redir string, hash []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the value from the "k" key in the HTTP POST
		k := r.PostFormValue("k")
		k = normalizeSubmission(k)
		if k == "" {
			// if the string is empty, abort early. This isn't a real guess.
			http.Redirect(w, r, indexURL, 302)
			return
		}
		log.Printf("q1 guess: %q\n", k)
		// split string on spaces
		sp := strings.Fields(k)
		// The reverse-proxy MUST handle this correctly.
		ip := r.Header.Get("X-Real-IP")
		if len(sp) > 1 {
			// gottem
			log.Println("good try fandom. cooldown ip for 5 minutes.")
			cooldown[ip] = time.Now().Add(5 * time.Minute)
		}
		if time.Now().Before(cooldown[ip]) {
			log.Println("buuusted!")
			// Dummy comparison
			bcrypt.CompareHashAndPassword(hash, []byte("buzz"))
			// Followed by an unconditional redirect to the index.
			http.Redirect(w, r, indexURL, 302)
			return
		}
		if err := bcrypt.CompareHashAndPassword(hash, []byte(k)); err != nil {
			// This is an easy one, really. You should get it the first guess.
			// If you don't, cooldown for 5 min.
			cooldown[ip] = time.Now().Add(5 * time.Minute)
			http.Redirect(w, r, indexURL, 302)
			return
		}
		// success case
		http.Redirect(w, r, redir, 302)
	}
}

func q2Handler(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := filepath.Base(file)
		t, err := template.New(f).Funcs(template.FuncMap{
			"noencode": func(s string) template.HTML { return template.HTML(s) },
		}).ParseFS(romFS, file)
		if err != nil {
			panic("internal error: could not template " + file)
		}
		err = t.ExecuteTemplate(w, f, struct {
			GUID string
		}{
			GUID: uuid.New().String(),
		})
		if err != nil {
			panic(err)
		}
	}
}

func q2SubmitHandler(hash []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.PostFormValue("k")
		k = normalizeSubmission(k)
		if k == "" {
			// if the string is empty, abort early. This isn't a real guess.
			http.Redirect(w, r, indexURL, 302)
			return
		}
		log.Printf("q2 guess: %q\n", k)
		ip := r.Header.Get("X-Real-IP")
		if time.Now().Before(cooldown[ip]) {
			log.Println("buuusted!")
			bcrypt.CompareHashAndPassword(hash, []byte("buzz"))
			http.Redirect(w, r, indexURL, 302)
			return
		}
		if err := bcrypt.CompareHashAndPassword(hash, []byte(k)); err != nil {
			// This is an easy one, really. You should get it the first guess.
			// If you don't, cooldown for 5 min.
			cooldown[ip] = time.Now().Add(5 * time.Minute)
			http.Redirect(w, r, indexURL, 302)
			return
		}
		// success case
		guid := uuid.New().String()
		finals[guid+ip] = true // if this were a *real* program, this implementation would be too naive.
		http.Redirect(w, r, "/final/"+guid+".html", 302)
	}
}

func q3Handler(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g := chi.URLParam(r, "guid")
		guid := uuid.MustParse(g)
		ip := r.Header.Get("X-Real-IP")
		if !finals[guid.String()+ip] {
			log.Println("naughty q3!")
			http.Redirect(w, r, indexURL, 302)
			return
		}
		// one minute timer here. It shouldn't take that.
		go func() {
			<-time.After(2 * time.Minute)
			finals[guid.String()+ip] = false
		}()
		f := filepath.Base(file)
		t, err := template.New(f).Funcs(template.FuncMap{
			"noencode": func(s string) template.HTML { return template.HTML(s) },
		}).ParseFS(romFS, file)
		if err != nil {
			panic("internal error: could not template " + file)
		}
		err = t.ExecuteTemplate(w, f, struct {
			GUID string
		}{
			GUID: guid.String(),
		})
		if err != nil {
			panic(err)
		}
	}
}

func q3SubmitHandler(redirURL string, hash []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.Header.Get("X-Real-IP")
		if time.Now().Before(cooldown[ip]) {
			log.Println("buuusted!")
			http.Redirect(w, r, indexURL, 302)
			return
		}
		k := r.PostFormValue("k")
		k = normalizeSubmission(k)
		if k == "" {
			// if the string is empty, abort early. This isn't a real guess.
			http.Redirect(w, r, indexURL, 302)
			return
		}
		k = strings.ReplaceAll(k, " ", "")
		log.Printf("q3 guess: %q\n", k)
		g := chi.URLParam(r, "guid")
		guid := uuid.MustParse(g)
		if !finals[guid.String()+ip] {
			log.Println("naughty q3 submission!")
			http.Redirect(w, r, indexURL, 302)
			return
		}
		finals[guid.String()+ip] = false // oneshot
		if err := bcrypt.CompareHashAndPassword(hash, []byte(k)); err != nil {
			// This is an easy one, really. You should get it the first guess.
			// If you don't, cooldown for 5 min.
			cooldown[ip] = time.Now().Add(5 * time.Minute)
			http.Redirect(w, r, indexURL, 302)
			return
		}
		// success case
		http.Redirect(w, r, redirURL, 302)
	}
}
