package utils

import (
	"encoding/csv"
	"log"
	"os"
	"io"
)

type Params struct {
	LogFile string
	InputFile string
	OutputFile string
	OutputColumns []string
}

type InputData struct {
	Line []string
	Writer *csv.Writer
	OutputFile *os.File
}


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

func openFile(path string, mode string) *os.File {
	var f *os.File
	var err error
	if mode == "read" {
		f, err = os.Open(path)
	} else {
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	if err != nil {
		log.Fatal(err)
	}

	return f
}

func CreateWriter(path string, columns []string) (*os.File, *csv.Writer) {
	f := openFile(path, "write")

	writer := csv.NewWriter(f)
	writer.Write(columns)
	return f, writer
}

func CreateReader(path string) (*os.File, *csv.Reader) {
	f := openFile(path, "read")

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
	f := openFile(path, "write")
	logger := log.New(f, "ErrorLogger: ", log.Ldate|log.Ltime|log.Lshortfile)
	return f, logger
}
