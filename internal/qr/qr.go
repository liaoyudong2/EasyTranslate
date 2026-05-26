package qr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

type versionInfo struct {
	version       int
	size          int
	dataCodewords int
	eccCodewords  int
}

var versions = []versionInfo{
	{version: 1, size: 21, dataCodewords: 19, eccCodewords: 7},
	{version: 2, size: 25, dataCodewords: 34, eccCodewords: 10},
	{version: 3, size: 29, dataCodewords: 55, eccCodewords: 15},
	{version: 4, size: 33, dataCodewords: 80, eccCodewords: 20},
	{version: 5, size: 37, dataCodewords: 108, eccCodewords: 26},
}

func PNG(text string, pixels int) ([]byte, error) {
	matrix, err := Encode(text)
	if err != nil {
		return nil, err
	}
	if pixels <= 0 {
		pixels = 256
	}
	quiet := 4
	modules := len(matrix) + quiet*2
	scale := pixels / modules
	if scale < 1 {
		scale = 1
	}
	size := modules * scale
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.RGBA{A: 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	for y, row := range matrix {
		for x, dark := range row {
			if !dark {
				continue
			}
			startX := (x + quiet) * scale
			startY := (y + quiet) * scale
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(startX+dx, startY+dy, black)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Encode(text string) ([][]bool, error) {
	data := []byte(text)
	var info versionInfo
	ok := false
	for _, candidate := range versions {
		if len(data)+2 <= candidate.dataCodewords {
			info = candidate
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("二维码内容过长")
	}

	dataCodewords := makeDataCodewords(data, info.dataCodewords)
	ecc := reedSolomonRemainder(dataCodewords, info.eccCodewords)
	allCodewords := append(dataCodewords, ecc...)

	modules := make([][]bool, info.size)
	function := make([][]bool, info.size)
	for i := range modules {
		modules[i] = make([]bool, info.size)
		function[i] = make([]bool, info.size)
	}

	drawFunctionPatterns(modules, function, info.version)
	drawCodewords(modules, function, allCodewords)
	drawFormatBits(modules, function, 0)
	return modules, nil
}

func makeDataCodewords(data []byte, capacity int) []byte {
	var bits []bool
	appendBits := func(value, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, ((value>>i)&1) != 0)
		}
	}
	appendBits(0b0100, 4)
	appendBits(len(data), 8)
	for _, b := range data {
		appendBits(int(b), 8)
	}
	remaining := capacity*8 - len(bits)
	if remaining > 4 {
		remaining = 4
	}
	for i := 0; i < remaining; i++ {
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}

	codewords := make([]byte, 0, capacity)
	for i := 0; i < len(bits); i += 8 {
		var value byte
		for j := 0; j < 8; j++ {
			value <<= 1
			if bits[i+j] {
				value |= 1
			}
		}
		codewords = append(codewords, value)
	}
	pads := []byte{0xEC, 0x11}
	for i := 0; len(codewords) < capacity; i++ {
		codewords = append(codewords, pads[i%2])
	}
	return codewords
}

func drawFunctionPatterns(modules, function [][]bool, version int) {
	size := len(modules)
	drawFinder(modules, function, 3, 3)
	drawFinder(modules, function, size-4, 3)
	drawFinder(modules, function, 3, size-4)

	for i := 0; i < size; i++ {
		if !function[6][i] {
			setFunction(modules, function, i, 6, i%2 == 0)
		}
		if !function[i][6] {
			setFunction(modules, function, 6, i, i%2 == 0)
		}
	}

	if version > 1 {
		centers := []int{6, size - 7}
		for _, y := range centers {
			for _, x := range centers {
				if function[y][x] {
					continue
				}
				drawAlignment(modules, function, x, y)
			}
		}
	}

	setFunction(modules, function, 8, size-8, true)
	for i := 0; i < 8; i++ {
		setFunction(modules, function, 8, i, false)
		setFunction(modules, function, i, 8, false)
		setFunction(modules, function, size-1-i, 8, false)
		setFunction(modules, function, 8, size-1-i, false)
	}
	setFunction(modules, function, 8, 8, false)
}

func drawFinder(modules, function [][]bool, cx, cy int) {
	size := len(modules)
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x := cx + dx
			y := cy + dy
			if x < 0 || y < 0 || x >= size || y >= size {
				continue
			}
			dist := max(abs(dx), abs(dy))
			dark := dist != 2 && dist != 4
			setFunction(modules, function, x, y, dark)
		}
	}
}

func drawAlignment(modules, function [][]bool, cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			dist := max(abs(dx), abs(dy))
			setFunction(modules, function, cx+dx, cy+dy, dist != 1)
		}
	}
}

func drawCodewords(modules, function [][]bool, codewords []byte) {
	size := len(modules)
	bitIndex := 0
	upward := true
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right--
		}
		for vertical := 0; vertical < size; vertical++ {
			y := vertical
			if upward {
				y = size - 1 - vertical
			}
			for dx := 0; dx < 2; dx++ {
				x := right - dx
				if function[y][x] {
					continue
				}
				dark := false
				if bitIndex < len(codewords)*8 {
					dark = ((codewords[bitIndex/8] >> (7 - bitIndex%8)) & 1) != 0
				}
				if (x+y)%2 == 0 {
					dark = !dark
				}
				modules[y][x] = dark
				bitIndex++
			}
		}
		upward = !upward
	}
}

func drawFormatBits(modules, function [][]bool, mask int) {
	size := len(modules)
	data := (1 << 3) | mask
	bits := formatBits(data)
	for i := 0; i <= 5; i++ {
		setFunction(modules, function, 8, i, bit(bits, i))
	}
	setFunction(modules, function, 8, 7, bit(bits, 6))
	setFunction(modules, function, 8, 8, bit(bits, 7))
	setFunction(modules, function, 7, 8, bit(bits, 8))
	for i := 9; i < 15; i++ {
		setFunction(modules, function, 14-i, 8, bit(bits, i))
	}
	for i := 0; i < 8; i++ {
		setFunction(modules, function, size-1-i, 8, bit(bits, i))
	}
	for i := 8; i < 15; i++ {
		setFunction(modules, function, 8, size-15+i, bit(bits, i))
	}
	setFunction(modules, function, 8, size-8, true)
}

func formatBits(data int) int {
	value := data << 10
	for i := 14; i >= 10; i-- {
		if ((value >> i) & 1) != 0 {
			value ^= 0x537 << (i - 10)
		}
	}
	return ((data << 10) | value) ^ 0x5412
}

func reedSolomonRemainder(data []byte, degree int) []byte {
	divisor := reedSolomonDivisor(degree)
	result := make([]byte, degree)
	for _, b := range data {
		factor := b ^ result[0]
		copy(result, result[1:])
		result[degree-1] = 0
		for i := range result {
			result[i] ^= gfMultiply(divisor[i], factor)
		}
	}
	return result
}

func reedSolomonDivisor(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range result {
			result[j] = gfMultiply(result[j], root)
			if j+1 < len(result) {
				result[j] ^= result[j+1]
			}
		}
		root = gfMultiply(root, 0x02)
	}
	return result
}

func gfMultiply(x, y byte) byte {
	var z int
	a := int(x)
	b := int(y)
	for b != 0 {
		if b&1 != 0 {
			z ^= a
		}
		a <<= 1
		if a&0x100 != 0 {
			a ^= 0x11D
		}
		b >>= 1
	}
	return byte(z)
}

func setFunction(modules, function [][]bool, x, y int, dark bool) {
	modules[y][x] = dark
	function[y][x] = true
}

func bit(value, index int) bool {
	return ((value >> index) & 1) != 0
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
