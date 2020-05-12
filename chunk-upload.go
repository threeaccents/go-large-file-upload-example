package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
)

const (
	maxChunkSize = int64(5 << 20) // 5MB
)

// Chunk is a chunk of a file.
// It contains information to be able to put the full file back together
// when all file chunks have been uploaded.
type Chunk struct {
	UploadID      string // unique id for the current upload.
	ChunkNumber   int32
	TotalChunks   int32
	TotalFileSize int64 // in bytes
	Filename      string
	Data          io.Reader
	UploadDir     string
}

func processChunk(r *http.Request) error {
	chunk, err := parseChunk(r)
	if err != nil {
		return fmt.Errorf("failed to parse chunk %w", err)
	}

	// Let's create the dir to store the file chunks.
	if err := os.MkdirAll(chunk.UploadID, 02750); err != nil {
		return err
	}

	if err := storeChunk(chunk); err != nil {
		return err
	}

	return nil
}

func parseChunk(r *http.Request) (*Chunk, error) {
	var chunk Chunk

	buf := new(bytes.Buffer)

	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}

	// start readings parts
	// 1. upload id
	// 2. chunk number
	// 3. total chunks
	// 4. total file size
	// 5. file name
	// 6. chunk data

	// 1
	if err := getPart("upload_id", reader, buf); err != nil {
		return nil, err
	}

	chunk.UploadID = buf.String()
	buf.Reset()

	// dir to where we store our chunk
	chunk.UploadDir = fmt.Sprintf("./data/chunks/%s", chunk.UploadID)

	// 2
	if err := getPart("chunk_number", reader, buf); err != nil {
		return nil, err
	}

	parsedChunkNumber, err := strconv.ParseInt(buf.String(), 10, 32)
	if err != nil {
		return nil, err
	}

	chunk.ChunkNumber = int32(parsedChunkNumber)
	buf.Reset()

	// 3
	if err := getPart("total_chunks", reader, buf); err != nil {
		return nil, err
	}

	parsedTotalChunksNumber, err := strconv.ParseInt(buf.String(), 10, 32)
	if err != nil {
		return nil, err
	}

	chunk.TotalChunks = int32(parsedTotalChunksNumber)
	buf.Reset()

	// 4
	if err := getPart("total_file_size", reader, buf); err != nil {
		return nil, err
	}

	parsedTotalFileSizeNumber, err := strconv.ParseInt(buf.String(), 10, 64)
	if err != nil {
		return nil, err
	}

	chunk.TotalFileSize = parsedTotalFileSizeNumber
	buf.Reset()

	// 5
	if err := getPart("file_name", reader, buf); err != nil {
		return nil, err
	}

	chunk.Filename = buf.String()
	buf.Reset()

	// 6
	part, err := reader.NextPart()
	if err != nil {
		return nil, fmt.Errorf("failed reading chunk part %w", err)
	}

	chunk.Data = part

	return &chunk, nil
}

func storeChunk(chunk *Chunk) error {
	chunkFile, err := os.Create(fmt.Sprintf("%s/%d", chunk.UploadDir, chunk.ChunkNumber))
	if err != nil {
		return err
	}

	if _, err := io.CopyN(chunkFile, chunk.Data, maxChunkSize); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func getPart(expectedPart string, reader *multipart.Reader, buf *bytes.Buffer) error {
	part, err := reader.NextPart()
	if err != nil {
		return fmt.Errorf("failed reading %s part %w", expectedPart, err)
	}

	if part.FormName() != expectedPart {
		return fmt.Errorf("invalid form name for part. Expected %s got %s", expectedPart, part.FormName())
	}

	if _, err := io.Copy(buf, part); err != nil {
		return fmt.Errorf("failed copying %s part %w", expectedPart, err)
	}

	return nil
}
