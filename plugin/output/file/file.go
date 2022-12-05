package file

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"telemetry/models"
	"telemetry/plugin/serializers"
)

type File struct {
	Files               []string `json:"files"`
	RotationInterval    Duration `json:"rotation_interval"`
	RotationMaxSize     Size     `json:"rotation_max_size"`
	RotationMaxArchives int      `json:"rotation_max_archives"`

	log *logrus.Entry

	writer     io.Writer
	closers    []io.Closer
	serializer serializers.Serializer
}

func (f *File) SetSerializer(serializer serializers.Serializer) {
	f.serializer = serializer
}

func NewFile() *File {
	return &File{
		log: models.NewLogger("outputs.file"),
	}
}

func (f *File) Connect() error {
	writers := []io.Writer{}

	if len(f.Files) == 0 {
		f.Files = []string{"stdout"}
	}

	for _, file := range f.Files {
		if file == "stdout" {
			writers = append(writers, os.Stdout)
		} else {
			of, err := NewFileWriter(file, time.Duration(f.RotationInterval), int64(f.RotationMaxSize), f.RotationMaxArchives)
			if err != nil {
				return err
			}

			writers = append(writers, of)
			f.closers = append(f.closers, of)
		}
	}
	f.writer = io.MultiWriter(writers...)
	return nil
}

func (f *File) Close() error {
	var err error
	for _, c := range f.closers {
		errClose := c.Close()
		if errClose != nil {
			err = errClose
		}
	}
	return err
}

func (f *File) Write(metrics []models.Metric) error {
	var writeErr error
	for _, metric := range metrics {
		b, err := f.serializer.Serialize(metric)
		if err != nil {
			f.log.Debugf("Could not serialize metric: %v", err)
		}

		_, err = f.writer.Write(b)
		if err != nil {
			writeErr = fmt.Errorf("failed to write message: %v", err)
		}
	}

	return writeErr
}

func (f *File) ParseConfig(cfg map[string]any) error {
	tmp, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(tmp, f)
	if err != nil {
		return fmt.Errorf("[file] config error: %v", err)
	}
	return nil
}
