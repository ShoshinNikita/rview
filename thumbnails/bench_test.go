package thumbnails

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkVipsthumbnail can be used to benchmark different vipsthumbnail settings.
// To set up the environment (download images and etc.), set 'prepare' to true and
// run the benchmark.
//
// One-liner:
//
//	go test -run="^\$" -bench="^BenchmarkVipsthumbnail\$" -v -count=10 -timeout=10m > _bench.txt && benchstat -row /file -col /format,/params _bench.txt
func BenchmarkVipsthumbnail(b *testing.B) {
	const prepare = false

	files := []BenchFile{
		// https://commons.wikimedia.org/wiki/File:Mount_Ararat_and_the_Yerevan_skyline_(June_2018).jpg
		{Name: "Mount_Ararat.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/4/45/Mount_Ararat_and_the_Yerevan_skyline_%28June_2018%29.jpg"},
		// https://commons.wikimedia.org/wiki/File:View_of_Mount_Fuji_from_%C5%8Cwakudani_20211202.jpg
		{Name: "Mount_Fuji.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/f/f8/View_of_Mount_Fuji_from_%C5%8Cwakudani_20211202.jpg"},
		// https://commons.wikimedia.org/wiki/File:Rustaveli_National_Theater_in_Georgia_(Europe),_built_19th_century_in_Rococo_style.jpg
		{Name: "Rustaveli_National_Theater.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/e/e6/Rustaveli_National_Theater_in_Georgia_%28Europe%29%2C_built_19th_century_in_Rococo_style.jpg"},
		// https://commons.wikimedia.org/wiki/File:Sainte_Chapelle_Interior_Stained_Glass.jpg
		{Name: "Sainte_Chapelle.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/3/36/Sainte_Chapelle_Interior_Stained_Glass.jpg"},
		// https://commons.wikimedia.org/wiki/File:Sky_over_Munich_02.jpg
		{Name: "Sky.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/e/eb/Sky_over_Munich_02.jpg"},
		// https://en.wikipedia.org/wiki/File:Hovhannes_Aivazovsky_-_The_Ninth_Wave_-_Google_Art_Project.jpg
		{Name: "The_Ninth_Wave.jpg", URL: "https://upload.wikimedia.org/wikipedia/commons/4/4a/Hovhannes_Aivazovsky_-_The_Ninth_Wave_-_Google_Art_Project.jpg"},
	}

	if prepare {
		err := os.MkdirAll("_data/resized", 0777)
		if err != nil {
			b.Fatalf("mkdir failed: %s", err)
		}

		for _, file := range files {
			log.Printf("download %q...", file.Name)
			if err := file.Download(); err != nil {
				b.Fatalf("couldn't download %q: %s", file.Name, err)
			}
		}

		b.Fatal("preparations are complete, set 'prepare' to false and run benchmark")
	}

	dataDir, err := filepath.Abs("./_data")
	if err != nil {
		b.Fatalf("couldn't get absolute path of ./_data: %s", err)
	}

	for _, bb := range []struct {
		ext     string
		size    string
		params  string
		threads int
	}{
		{ext: ".jpeg", size: "1024>", params: "[Q=80,optimize_coding,keep=icc]", threads: 0},
		{ext: ".avif", size: "1024>", params: "[Q=65,speed=8,keep=icc]", threads: 0},
	} {
		format := strings.TrimPrefix(path.Ext(bb.ext), ".")
		name := fmt.Sprintf("format=%s/params=%s/size=%s/threads=%d", format, bb.params, bb.size, bb.threads)

		b.Run(name, func(b *testing.B) {
			for _, file := range files {
				b.Run("file="+file.Name, func(b *testing.B) {
					for b.Loop() {
						b.StopTimer()
						output := filepath.Join("resized", file.Name+bb.ext+bb.params)
						cmd := exec.Command("vipsthumbnail", file.Name, "--rotate", "--size", bb.size, "-o", output) //nolint:gosec
						cmd.Dir = dataDir
						if bb.threads > 0 {
							cmd.Env = append(cmd.Env, fmt.Sprintf("VIPS_CONCURRENCY=%d", bb.threads))
						}
						b.StartTimer()

						if err := cmd.Run(); err != nil {
							b.Fatalf("vipsthumbnail failed: %s", err)
						}
					}

					resizedFile := filepath.Join("resized", file.Name+bb.ext)
					stats, err := os.Stat(filepath.Join("_data", resizedFile))
					if err != nil {
						b.Fatalf("os.Stat failed: %s", err)
					}
					b.ReportMetric(float64(stats.Size()), "B")
				})
			}
		})
	}
}

type BenchFile struct {
	Name string
	URL  string
}

func (f BenchFile) Download() error {
	resp, err := http.Get(f.URL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("got invalid status code: %d", resp.StatusCode)
	}

	path := filepath.Join("_data", f.Name)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("couldn't create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("couldn't copy body to file: %w", err)
	}
	return nil
}
