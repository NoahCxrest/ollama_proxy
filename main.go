package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var ollamaURL string

func init() {
	ollamaURL = os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
}

func main() {
	http.HandleFunc("/api/", proxyHandler)
	fmt.Println("Proxy server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Incoming request: %s %s", r.Method, r.URL.Path)
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusOK)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var reqData map[string]interface{}
	if err := json.Unmarshal(body, &reqData); err != nil {
		log.Printf("Error unmarshaling request: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if images, ok := reqData["images"].([]interface{}); ok {
		for i, img := range images {
			if imgStr, ok := img.(string); ok {
				cleaned := strings.ReplaceAll(imgStr, "\n", "")
				cleaned = strings.ReplaceAll(cleaned, "\r", "")
				cleaned = strings.ReplaceAll(cleaned, " ", "")
				if _, err := base64.StdEncoding.DecodeString(cleaned); err != nil {
					log.Printf("Invalid base64 in image %d: %v", i, err)
					http.Error(w, "Invalid base64 in images", http.StatusBadRequest)
					return
				}
				images[i] = cleaned
			}
		}
		reqData["images"] = images
	}

	body, err = json.Marshal(reqData)
	if err != nil {
		log.Printf("Error re-marshaling request: %v", err)
		http.Error(w, "Failed to process request", http.StatusInternalServerError)
		return
	}

	url := ollamaURL + r.URL.Path
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}
	log.Printf("Proxying request t: %s", url)
	req, err := http.NewRequest(r.Method, url, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error proxying request to %s: %v", url, err)
		http.Error(w, "Failed to proxy request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
