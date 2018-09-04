package main

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/sirupsen/logrus"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		userAgent := r.Header.Get("User-Agent")
		logrus.Infof("%s - [%s %s %s] 200 [%s]", r.RemoteAddr, r.Method, r.URL.Path, r.Proto, userAgent)
		rn := rand.Intn(100)
		if rn > 50 {
			w.Header().Set("Server", "Hello/v2")
			fmt.Fprintf(w, "Hello Kubernetes! V2 (Bad)\n")
		} else {
			logrus.Errorf("Error %s - [%s %s %s] 500 [%s]", r.RemoteAddr, r.Method, r.URL.Path, r.Proto, userAgent)
			http.Error(w, "oops.... We go an Error! V2 (Bad)", 500)
			return
		}
	})

	logrus.SetFormatter(&FluentdFormatter{})
	logrus.Info("Starting Hello-Kubernetes service V2 (Bad)...")
	logrus.Fatal(http.ListenAndServe(":8080", nil))
}
