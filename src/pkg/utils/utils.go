package utils

import (
	"encoding/csv"
	"log"
	"os"
	"io"
)

func IsInList(target string, list []string) bool {
	for _, element := range list {
		if element == target {
			return true
		}
	}
	return false
}

func WriteComments(writer *csv.Writer, dataCh chan []string, done chan bool) {
	data := <- dataCh

	if len(data) > 0 {
		err := writer.Write(data)
		if err != nil {
			log.Fatal(err)
		}
	}
	
	done <- true
}

func openFile(path string) *os.File {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	return f
}

func CreateWriter(path string, columns []string) (*os.File, *csv.Writer) {
	f := openFile(path)

	writer := csv.NewWriter(f)
	writer.Write(columns)
	return f, writer
}

func CreateReader(path string) (*os.File, *csv.Reader) {
	f := openFile(path)

	csvReader := csv.NewReader(f)

	_, err := csvReader.Read() // skip first line
	if err != nil {
		if err != io.EOF {
			log.Fatalln(err)
		}
	}

	return f, csvReader
}

func CreateLogger(path string) (*os.File, *log.Logger) {
	f := openFile(path)
	logger := log.New(f, "ErrorLogger: ", log.Ldate|log.Ltime|log.Lshortfile)
	return f, logger
}
