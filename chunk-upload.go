package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"strconv"
)

const (
	maxChunkSize = int64(5 << 20) // 5MB

	uploadDir = "./data/chunks"
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

// ProcessChunk will parse the chunk data from the request and store in a file on disk.
func ProcessChunk(r *http.Request) error {
	chunk, err := ParseChunk(r)
	if err != nil {
		return fmt.Errorf("failed to parse chunk %w", err)
	}

	// Let's create the dir to store the file chunks.
	if err := os.MkdirAll(chunk.UploadID, 02750); err != nil {
		return err
	}

	if err := StoreChunk(chunk); err != nil {
		return err
	}

	return nil
}

// CompleteChunk rebulds the file chunks into the original full file.
// It then stores the file on disk.
func CompleteChunk(uploadID, filename string) error {
	uploadDir := fmt.Sprintf("%s/%s", uploadDir, uploadID)

	f, err := RebuildFile(uploadDir)
	if err != nil {
		return fmt.Errorf("failed to rebuild file %w", err)
	}
	// we might also want to delete the temp file from disk.
	// It would be handy to create a struct with a close method that closes and deletes the temp file.
	defer f.Close()

	// here we can just keep our file on disk
	// or do any processing we want such as resizing, tagging, storing in a cloud storage.
	// to keep this simple we'll just store the file on disk.

	newFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed creating file %w", err)
	}
	defer newFile.Close()

	if _, err := io.Copy(newFile, f); err != nil {
		return fmt.Errorf("failed copying file contents %w", err)
	}

	return nil
}

// ParseChunk parse the request body and creates our chunk struct. It expects the data to be sent in a
// specific order and handles validating the order.
func ParseChunk(r *http.Request) (*Chunk, error) {
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
	chunk.UploadDir = fmt.Sprintf("%s/%s", uploadDir, chunk.UploadID)

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

// StoreChunk stores the chunk on disk for it to later be processed when all other file chunks have been uploaded.
func StoreChunk(chunk *Chunk) error {
	chunkFile, err := os.Create(fmt.Sprintf("%s/%d", chunk.UploadDir, chunk.ChunkNumber))
	if err != nil {
		return err
	}

	if _, err := io.CopyN(chunkFile, chunk.Data, maxChunkSize); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// ByChunk is a helper type to sort the files by name. Since the name of the file is it's chunk number
// it makes rebuilding the file a trivial task.
type ByChunk []os.FileInfo

func (a ByChunk) Len() int      { return len(a) }
func (a ByChunk) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByChunk) Less(i, j int) bool {
	ai, _ := strconv.Atoi(a[i].Name())
	aj, _ := strconv.Atoi(a[j].Name())
	return ai < aj
}

// RebuildFile grabs all the files from the directory passed on concantinates them to build the original file.
// It stores the file contents in a temp file and returns it.
func RebuildFile(dir string) (*os.File, error) {
	fileInfos, err := ioutil.ReadDir(uploadDir)
	if err != nil {
		return nil, err
	}

	fullFile, err := ioutil.TempFile("", "fullfile-")
	if err != nil {
		return nil, err
	}

	sort.Sort(ByChunk(fileInfos))
	for _, fs := range fileInfos {
		if err := appendChunk(uploadDir, fs, fullFile); err != nil {
			return nil, err
		}
	}

	if err := os.RemoveAll(uploadDir); err != nil {
		return nil, err
	}

	return fullFile, nil
}

func appendChunk(uploadDir string, fs os.FileInfo, fullFile *os.File) error {
	src, err := os.Open(uploadDir + "/" + fs.Name())
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err := io.Copy(fullFile, src); err != nil {
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
