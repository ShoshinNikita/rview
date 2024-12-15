package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

// TestDataModTimes contains all files in "testdata" directory and their mod times.
var TestDataModTimes = map[string]string{
	"archive.7z":      "2022-04-07 05:23:55",
	"Lorem ipsum.txt": "2023-02-27 15:00:00",
	"main.go":         "2022-04-07 18:23:55",
	"test.gif":        "2023-01-01 15:00:00",
	//
	"Audio/":                        "2022-08-09 00:15:30",
	"Audio/click-button-140881.mp3": "2022-08-09 00:15:30",
	"Audio/credits.txt":             "2022-08-09 00:15:38",
	//
	"Images/":                         "2023-01-01 18:35:00",
	"Images/Photos/":                  "2023-01-01 18:35:00",
	"Images/Arts/":                    "2023-01-01 18:35:00",
	"Images/birds-g64b44607c_640.jpg": "2019-05-15 06:30:09",
	"Images/corgi-g4ea377693_640.jpg": "2023-01-01 18:35:00",
	"Images/credits.txt":              "2023-01-01 18:36:00",
	"Images/horizontal.jpg":           "2023-01-01 15:00:00",
	"Images/vertical.jpg":             "2023-01-01 15:00:00",
	"Images/zebra-g4e368da8d_640.jpg": "2023-01-05 16:00:37",
	"Images/qwerty.webp":              "2023-05-19 18:33:12",
	"Images/ytrewq.png":               "2023-05-19 18:33:12",
	//
	"Video/":                  "2022-09-08 11:37:02",
	"Video/credits.txt":       "2022-09-08 11:37:12",
	"Video/traffic-53902.mp4": "2022-09-08 11:37:02",
	"Video/boat-153559.mp4":   "2023-06-04 11:37:02",
	//
	"Other/": "2022-09-08 11:37:02",
	"Other/spe'sial ! cha<racters/x/y/f>ile.txt":      "2022-09-08 11:37:02",
	"Other/test-thumbnails/cloudy-g1a943401b_640.png": "2022-09-11 18:35:04",
	"Other/test-thumbnails/credits.txt":               "2022-09-11 18:35:04",
	"Other/a & b/x/x & y.txt":                         "2023-06-06 00:00:13",
	"Other/Double\" quote.txt":                        "2024-12-15 23:00:00",
	"Other/Single' quote.txt":                         "2024-12-15 23:00:00",
}

func TestMain(m *testing.M) {
	for path, rawModTime := range TestDataModTimes {
		modTime, err := time.Parse(time.DateTime, rawModTime)
		if err != nil {
			panic(fmt.Errorf("couldn't parse testdata mod time: %w", err))
		}

		modTime = modTime.UTC()
		err = os.Chtimes("testdata/"+path, modTime, modTime)
		if err != nil {
			panic(fmt.Errorf("couldn't change mod time of %q: %w", path, err))
		}
	}

	code := m.Run()

	// Shutdown test rview if needed.
	if testRview != nil {
		ctx, _ := context.WithTimeout(context.Background(), 2*time.Second) //nolint:govet

		err := testRview.Shutdown(ctx)
		if err != nil {
			code = 1
			log.Printf("shutdown error: %s", err)
		}

		<-testRviewDone
	}

	os.Exit(code)
}
