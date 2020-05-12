package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/upload-chunk", uploadChunkHandler)

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func uploadChunkHandler(w http.ResponseWriter, r *http.Request) {
	if err := processChunk(r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("chunk processed"))
}
