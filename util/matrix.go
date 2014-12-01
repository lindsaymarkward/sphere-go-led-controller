package util

import (
	"fmt"
	"image"
	"io"
	"math"
	"os/exec"
	"time"
	"strings"

	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/goserial"
)

var log = logger.GetLogger("led-matrix")

// Attempts this first, then falls back to half.
const baudRate = 230400

var cmdWriteBuffer byte = 1
var cmdSwapBuffers byte = 2

// From https://diarmuid.ie/blog/post/pwm-exponential-led-fading-on-arduino-or-other-platforms
var R = (255 * math.Log10(2)) / (math.Log10(255))
var ledAdjust = make(map[uint8]uint8)

func init() {
	for i := 0; i < 256; i++ {
		ledAdjust[uint8(i)] = uint8(math.Pow(2, (float64(i)/R)) - 1)
	}
}

func GetLEDConnectionAtRate(baudRate int) (io.ReadWriteCloser, error) {

	c := &serial.Config{Name: "/dev/tty.ledmatrix", Baud: baudRate}
	s, err := serial.OpenPort(c)
	if err != nil {
		return nil, err
	}
	
	// Wait a little bit, to make sure we actually receive the startup data, and in one go
	time.Sleep(time.Millisecond * 500)

	// Now we wait for the init string
	buf := make([]byte, 16)
	_, err = s.Read(buf)
	if err != nil {
		log.Fatalf("Failed to read initialisation string from led matrix : %s", err)
	}
	
	// on 3.12 we get a \x00 before the 'LED' init string, so we just check for existance now
	initString := string(buf)
	if strings.Contains(initString, "LED") {
		log.Infof("Expected init string to contain 'LED', got '%s'.", initString)
		s.Close()
		return nil, fmt.Errorf("Bad init string..")
	}

	log.Debugf("Read init string from LED Matrix: %s", buf)

	return s, nil
}

func GetLEDConnection() (io.ReadWriteCloser, error) {

	log.Debugf("Resetting LED Matrix")
	resetLedMatrix, err := exec.LookPath("reset-led-matrix")
	if err != nil {
	   return nil, err
	}
	cmd := exec.Command(resetLedMatrix)
	output, err := cmd.Output()
	log.Debugf("Output from reset: %s", output)

	s, err := GetLEDConnectionAtRate(baudRate)

	if err != nil {
		log.Warningf("Failed to connect to LED using baud rate: %d, trying %d. error:%s", baudRate, baudRate/2, err)

		for _, d := range []int{1, 2, 4} {
			s, err = GetLEDConnectionAtRate(baudRate / 2)
			if err == nil {
				break
			}
			if d == 4 {
				log.Fatalf("Failed to connect to LED display: %s", err)
			} else {
				time.Sleep(time.Second * time.Duration(d))
			}
		}
	}

	return s, err
}

func ConvertImage(image *image.RGBA) []byte {

	var frame [768]byte

	for inPos, outPos := 0, 0; inPos < len(image.Pix); inPos = inPos + 4 {

		outPos = inPos / 4 * 3

		frame[outPos] = ledAdjust[image.Pix[inPos]]
		frame[outPos+1] = ledAdjust[image.Pix[inPos+1]]
		frame[outPos+2] = ledAdjust[image.Pix[inPos+2]]
	}

	rows := split(frame[:], 16*3)

	var orderedRows [][]byte
	for i := 0; i < 8; i++ {
		orderedRows = append(orderedRows, rows[i+8])
		orderedRows = append(orderedRows, rows[i])
	}

	var finalFrame []byte

	for _, line := range orderedRows {
		for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
			line[i], line[j] = line[j], line[i]
		}

		finalFrame = append(finalFrame, line...)
	}

	return finalFrame
}

// Write an image into the led matrix
func WriteLEDMatrix(image *image.RGBA, s io.ReadWriteCloser) {

	//spew.Dump("writing image", image)

	finalFrame := ConvertImage(image)

	_, err := s.Write([]byte{cmdWriteBuffer})
	if err != nil {
		log.Fatalf("Failed writing write buffer command: %s", err)
	}

	_, err = s.Write(finalFrame[:])
	if err != nil {
		log.Fatalf("Failed writing frame: %s", err)
	}

	_, err = s.Write([]byte{cmdSwapBuffers})
	if err != nil {
		log.Fatalf("Failed writing swap buffer command: %s", err)
	}

	//log.Println("Wrote frame", n)
	buf := make([]byte, 1)
	_, err = s.Read(buf)
	if err != nil {
		log.Infof("Failed to read char after sending frame : %s", err)
	}
	if buf[0] != byte('F') {
		log.Infof("Expected an 'F', got '%q'", buf[0])
	}
}

func split(a []byte, size int) [][]byte {
	var out [][]byte
	var i = 0
	for i < len(a) {
		out = append(out, a[i:i+size])
		i += size
	}

	return out
}

// Simple RLE on zero values....
/*
func compress(frame []byte) []byte {
	compressed := make([]byte, 0)
	for i := 0; i < len(frame); i++ {

		val := frame[i]
		if val == 0 {

			count := 0
			for j := i + 1; j < len(frame) && frame[j] == val; j++ {
				count++
			}

			compressed = append(compressed, val, byte(count))
			i += count
		} else {
			compressed = append(compressed, val)
		}
	}
	//spew.Dump("from", frame, compressed)
	return compressed
}*/
