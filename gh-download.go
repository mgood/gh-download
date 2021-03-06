package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"code.google.com/p/goauth2/oauth"
	"github.com/google/go-github/github"
	"github.com/gorilla/mux"
)

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func marshal(obj interface{}) []byte {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		log.Println("marshal:", err)
	}
	return bytes
}

func normalVersion(version string) string {
	if version[0] == 'v' {
		return version[1:]
	}
	return version
}

func expandVersion(releases []github.RepositoryRelease, version string) string {
	var release github.RepositoryRelease
	if version == "latest" {
		release = releases[0]
	} else {
		for _, r := range releases {
			if r.TagName != nil && *r.TagName == version {
				release = r
				break
			}
		}
	}
	if release.TagName == nil {
		return ""
	}
	return *release.TagName
}

func proxyDownload(w http.ResponseWriter, url string) {
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		log.Println("error:", err)
		return
	}
	if resp.StatusCode == 200 {
		log.Println("download:", url)
	} else {
		log.Println("error:", resp.Status, url)
	}
	w.Header().Set("Backend-Url", url)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

func main() {
	port := os.Getenv("PORT")
	owner := os.Getenv("GITHUB_OWNER")

	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	}

	client := github.NewClient(t.Client())

	r := mux.NewRouter()

	r.HandleFunc("/{repo}/latest/version.txt", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		releases, _, err := client.Repositories.ListReleases(owner, vars["repo"], nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			log.Println("error:", err)
			return
		}
		io.WriteString(w, normalVersion(expandVersion(releases, "latest"))+"\n")
	})

	r.HandleFunc("/{repo}/{tag}.{ext}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		releases, _, err := client.Repositories.ListReleases(owner, vars["repo"], nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			log.Println("error:", err)
			return
		}
		version := expandVersion(releases, vars["tag"])
		w.Header().Set("Version", version)
		if version == "" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		proxyDownload(w, fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
			owner,
			vars["repo"],
			version,
			fmt.Sprintf("%s_%s.%s",
				vars["repo"],
				version,
				vars["ext"])))
	})

	r.HandleFunc("/{repo}/{tag}/{platform}.{ext}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		releases, _, err := client.Repositories.ListReleases(owner, vars["repo"], nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			log.Println("error:", err)
			return
		}
		version := expandVersion(releases, vars["tag"])
		w.Header().Set("Version", version)
		if version == "" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Platform", vars["platform"])
		platform := strings.SplitN(strings.ToLower(vars["platform"]), "_", 2)
		if len(platform) < 2 {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		proxyDownload(w, fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
			owner,
			vars["repo"],
			version,
			fmt.Sprintf("%s_%s_%s_%s.%s",
				vars["repo"],
				normalVersion(version),
				platform[0],
				platform[1],
				vars["ext"])))
	})

	http.Handle("/", r)
	log.Println("serving on port", port, "for", owner, "...")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
