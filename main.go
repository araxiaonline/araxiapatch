package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"
	"archive/tar"
	"compress/gzip"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/widgets"
)

type ProgressBarWindow struct {
	app          *widgets.QApplication
	window       *widgets.QWidget
	layout       *widgets.QVBoxLayout
	bars         []*ProgressBar
	maxNameWidth int
	done         chan bool
}

type ProgressBar struct {
	order       int
	total       int64
	current     int64
	file        string
	progressBar *widgets.QProgressBar
	label       *widgets.QLabel
}

// Files to download
var files = []string{
	"info.txt",
	"AraxiaPatchv1.tar.gz",
	"HDPatchv1.tar.gz",
}

var patchSource = "https://storage.googleapis.com/araxia-client-patches/Updatev1/"
var appName = "Araxia Client Patch Downloader"

func main() {
	catchInterrupt()

	app := widgets.NewQApplication(len(os.Args), os.Args)
	// Window setup
	window := widgets.NewQWidget(nil, 0)
	window.SetWindowTitle(appName)
	title := widgets.NewQLabel2(appName, nil, 0)
	title.Font().SetPointSize(20)
	title.Font().SetFamily("Arial")
	title.SetAlignment(core.Qt__AlignCenter)
	window.SetMinimumSize2(800, 600)

	// Build layout
	layout := widgets.NewQVBoxLayout()
	window.SetLayout(layout)
	layout.AddWidget(title, 0, core.Qt__AlignCenter)

	progressBarWindow := ProgressBarWindow{
		app:    app,
		window: window,
		layout: layout,
		done:   make(chan bool),
	}

	progressBarWindow.calculateMaxNameWidth()
	progressBarWindow.initProgressBars()

	go progressBarWindow.run()

	closeButton := widgets.NewQPushButton2("Close", nil)
	closeButton.ConnectClicked(func(bool) {
		app.Quit()
	})
	layout.AddWidget(closeButton, 0, core.Qt__AlignRight)

	window.Show()

	app.Exec()
}

func (p *ProgressBarWindow) calculateMaxNameWidth() {
	for _, file := range files {
		if len(file) > p.maxNameWidth {
			p.maxNameWidth = len(file)
		}
	}
}

func (p *ProgressBarWindow) initProgressBars() {
	for i, file := range files {
		progressBar := NewProgressBar(i+1, file, p.maxNameWidth)
		p.bars = append(p.bars, progressBar)

		// Create labels for filename and download speed
		filenameLabel := widgets.NewQLabel2(file, nil, 0)
		filenameLabel.SetFixedWidth(p.maxNameWidth * 8)

		// Create a horizontal layout for the labels and progress bar
		labelLayout := widgets.NewQHBoxLayout2(nil)
		labelLayout.AddWidget(filenameLabel, 0, core.Qt__AlignTop)
		labelLayout.AddWidget(progressBar.label, 0, core.Qt__AlignTop)

		// Create a vertical layout to hold the labels and progress bar
		progressLayout := widgets.NewQVBoxLayout()
		progressLayout.AddLayout(labelLayout, 0)
		progressLayout.AddWidget(progressBar.progressBar, 0, core.Qt__AlignTop)

		p.layout.AddLayout(progressLayout, 0)
	}
}

func (p *ProgressBarWindow) run() {
	directory := "."
	if len(os.Args) > 1 {
		directory = os.Args[1]
	}

	// create channel to wait for all downloads to finish
	done := make(chan bool)

	// Download each file in parallel
	for i, file := range files {
		go p.downloadFile(directory, file, i+1, done)
	}

	// Wait for all downloads to finish
	for i := 0; i < len(files); i++ {
		<-done
	}

	// Untar gz the patch files
	for _, file := range files {
		fmt.Println("Untarring", file)
		err := untarGz(directory+"/"+file, directory)
		if err != nil {
			fmt.Println("Error untarring file:", file, err)
		}
	}

}

func untarGz(src string, dest string) error {
	// Open gzip file
	gzipFile, err := os.Open(src)
	if err != nil {
		return err
	}

	// Check if file has tar.gz extension if not skip the file
	if gzipFile.Name()[len(gzipFile.Name())-6:] != ".tar.gz" {
		return nil
	}

	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)

	// Iterate through the files in the archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest+"/"+header.Name, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(dest + "/" + header.Name)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
			outFile.Close()
		default:
			fmt.Printf("Unable to untar type : %c in file %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

func (p *ProgressBarWindow) downloadFile(directory string, file string, order int, done chan bool) {
	out, err := os.Create(directory + "/" + file)
	if err != nil {
		fmt.Println("Error creating file:", file)
		return
	}
	defer out.Close()

	resp, err := http.Get(patchSource + file)
	if err != nil {
		fmt.Println("Error downloading file:", file)
		return
	}
	defer resp.Body.Close()

	progressBar := p.bars[order-1]
	progressBar.total = resp.ContentLength

	start := time.Now()
	lastTime := start
	lastBytes := int64(0)

	buf := make([]byte, 1024) // Buffer for calculating download speed
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			progressBar.current += int64(n)
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			if elapsed >= 1 { // Update speed label every second
				speed := float64(progressBar.current-lastBytes) / elapsed
				updateSpeedLabel(progressBar.label, speed)
				lastBytes = progressBar.current
				lastTime = now
			}
			updateProgressBar(progressBar.progressBar, progressBar.current, progressBar.total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error writing file:", file, err)
			return
		}
	}

	done <- true
}

func NewProgressBar(order int, file string, maxNameWidth int) *ProgressBar {
	progressBar := widgets.NewQProgressBar(nil)
	progressBar.SetMinimum(0)
	progressBar.SetMaximum(100)
	progressBar.SetValue(0)

	label := widgets.NewQLabel2("", nil, 0)
	label.SetFixedWidth(maxNameWidth * 8)

	return &ProgressBar{
		order:       order,
		file:        file,
		progressBar: progressBar,
		label:       label,
	}
}

func updateProgressBar(progressBar *widgets.QProgressBar, current int64, total int64) {
	percent := float32(current) / float32(total) * 100
	progressBar.SetValue(int(percent))
}

func updateSpeedLabel(label *widgets.QLabel, speed float64) {
	var speedLabel string
	if speed < 1024 {
		speedLabel = fmt.Sprintf("%.2f B/s", speed)
	} else if speed < 1024*1024 {
		speedLabel = fmt.Sprintf("%.2f KB/s", speed/1024)
	} else {
		speedLabel = fmt.Sprintf("%.2f MB/s", speed/1024/1024)
	}
	label.SetText(speedLabel)
}

// catch interrupt signal and exit
func catchInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Printf("Received %v, exiting.\n", sig)
			os.Exit(1)
		}
	}()
}
