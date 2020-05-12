package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.Handle("/upload-chunk", handleUploadChunk())
	http.Handle("/completed-chunks", handleCompletedChunk())

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleUploadChunk() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ProcessChunk(r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("chunk processed"))
	})
}
func handleCompletedChunk() http.Handler {
	type request struct {
		UploadID string `json:"uploadId"`
		Filename string `json:"filename"`
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// validate payload

		if err := CompleteChunk(payload.UploadID, payload.Filename); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("file processed"))
	})
}
