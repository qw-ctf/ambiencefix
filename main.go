package main

import (
	"encoding/binary"
	"io"
	"log"
	"math"
	"os"
	"strconv"
)

type WAVHeader struct {
	ChunkID   [4]byte
	ChunkSize int32
	Format    [4]byte
}

type FmtChunk struct {
	ChunkID       [4]byte
	ChunkSize     int32
	AudioFormat   int16
	NumChannels   int16
	SampleRate    int32
	ByteRate      int32
	BlockAlign    int16
	BitsPerSample int16
}

type ChunkHeader struct {
	ID   [4]byte
	Size int32
}

type CuePoint struct {
	Identifier   int32
	Position     uint32
	ChunkID      [4]byte
	ChunkStart   int32
	BlockStart   int32
	SampleOffset uint32
}

type CueChunk struct {
	ChunkID     [4]byte
	ChunkSize   int32
	DwCuePoints int32
	CuePoints   [1]CuePoint
}

type ListChunk struct {
	ChunkID   [4]byte
	ChunkSize int32
	FormType  [4]byte
}

type LtxtChunk struct {
	ChunkID        [4]byte
	ChunkSize      int32
	DwName         uint32
	DwSampleLength uint32
	DwPurpose      [4]byte
	DwCountry      int16
	DwLanguage     int16
	DwDialect      int16
	DwCodePage     int16
}

type LablChunk struct {
	ChunkID   [4]byte
	ChunkSize int32
	CueID     int32
	LabelText [8]byte
}

type NoteChunk struct {
	ChunkID   [4]byte
	ChunkSize int32
	CueID     int32
	NoteText  [6]byte
}

func findChunk(r *io.SectionReader, chunkID string) (io.ReadSeeker, int32, error) {
	var header ChunkHeader
	var position int64 = 0

	_, err := r.Seek(12, io.SeekStart) // skip RIFF header
	if err != nil {
		return nil, 0, err
	}
	position = 12

	for {
		err = binary.Read(r, binary.LittleEndian, &header)
		if err != nil {
			return nil, 0, err
		}

		if string(header.ID[:]) == chunkID {
			length := header.Size + int32(binary.Size(header)) + 1
			return io.NewSectionReader(r, position, int64(length)), length, nil
		}

		_, err = r.Seek(int64(header.Size), io.SeekCurrent)
		if err != nil {
			return nil, 0, err
		}

		position += int64(header.Size) + int64(binary.Size(header))
	}
}

func checkErr(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

func main() {
	if len(os.Args) < 4 {
		log.Fatalf("Usage: %s <offset-seconds> <input.wav> <output.wav>\n", os.Args[0])
	}

	offsetSeconds, err := strconv.ParseFloat(os.Args[1], 64)
	checkErr(err, "Could not parse timestamp")

	inputFilename := os.Args[2]
	outputFilename := os.Args[3]

	input, err := os.Open(inputFilename)
	checkErr(err, "Could not open input file")
	defer input.Close()

	header := WAVHeader{}
	err = binary.Read(input, binary.LittleEndian, &header)
	checkErr(err, "Could not read WAV header")

	fileInfo, err := input.Stat()
	checkErr(err, "Could not stat file")

	r := io.NewSectionReader(input, 0, fileInfo.Size())

	fmtReader, _, err := findChunk(r, "fmt ")
	checkErr(err, "Could not find fmt chunk")

	var fmtChunk FmtChunk
	err = binary.Read(fmtReader, binary.LittleEndian, &fmtChunk)
	checkErr(err, "Could not read fmt chunk")

	dataReader, dataChunkSize, err := findChunk(r, "data")
	checkErr(err, "Could not find data chunk")

	output, err := os.Create(outputFilename)
	checkErr(err, "Could not create output file")
	defer output.Close()

	err = binary.Write(output, binary.LittleEndian, &header)
	checkErr(err, "Could not write WAV header")

	err = binary.Write(output, binary.LittleEndian, &fmtChunk)
	checkErr(err, "Could not write fmt chunk")

	chunkHeaderSize := int32(binary.Size(ChunkHeader{}))

	cuePosition := uint32(math.Floor(offsetSeconds * float64(fmtChunk.SampleRate) * float64(fmtChunk.NumChannels)))
	dataSize := dataChunkSize - chunkHeaderSize

	_, err = io.CopyN(output, dataReader, int64(dataChunkSize))
	checkErr(err, "Could not write data chunk")

	cuePoint := CuePoint{
		Identifier:   1,
		Position:     cuePosition,
		ChunkID:      [4]byte{'d', 'a', 't', 'a'},
		ChunkStart:   0,
		BlockStart:   0,
		SampleOffset: cuePosition,
	}

	cue := CueChunk{
		ChunkID:     [4]byte{'c', 'u', 'e', ' '},
		ChunkSize:   int32(binary.Size(CueChunk{})) - chunkHeaderSize,
		DwCuePoints: 1,
		CuePoints:   [1]CuePoint{cuePoint},
	}

	err = binary.Write(output, binary.LittleEndian, &cue)
	checkErr(err, "Could not write Cue chunk")

	note := NoteChunk{
		ChunkID:   [4]byte{'n', 'o', 't', 'e'},
		ChunkSize: int32(binary.Size(NoteChunk{})) - chunkHeaderSize,
		CueID:     1,
		NoteText:  [6]byte{'R', 'a', 'n', 'g', 'e', 0},
	}

	labl := LablChunk{
		ChunkID:   [4]byte{'l', 'a', 'b', 'l'},
		ChunkSize: int32(binary.Size(LablChunk{})) - chunkHeaderSize,
		CueID:     1,
		LabelText: [8]byte{'M', 'A', 'R', 'K', '0', '0', '1', 0},
	}

	ltxt := LtxtChunk{
		ChunkID:        [4]byte{'l', 't', 'x', 't'},
		ChunkSize:      int32(binary.Size(LtxtChunk{})) - chunkHeaderSize,
		DwName:         cuePosition,
		DwSampleLength: uint32(dataSize) - cuePosition - 1,
		DwPurpose:      [4]byte{'m', 'a', 'r', 'k'},
		DwCountry:      1,
		DwLanguage:     0,
		DwDialect:      0,
		DwCodePage:     0,
	}

	listChunk := ListChunk{
		ChunkID:   [4]byte{'L', 'I', 'S', 'T'},
		ChunkSize: int32(binary.Size(ListChunk{})) - chunkHeaderSize + int32(binary.Size(ltxt)+binary.Size(labl)+binary.Size(note)),
		FormType:  [4]byte{'a', 'd', 't', 'l'},
	}

	err = binary.Write(output, binary.LittleEndian, &listChunk)
	checkErr(err, "Could not write list chunk")

	err = binary.Write(output, binary.LittleEndian, &ltxt)
	checkErr(err, "Could not write ltxt chunk")

	err = binary.Write(output, binary.LittleEndian, &labl)
	checkErr(err, "Could not write labl chunk")

	err = binary.Write(output, binary.LittleEndian, &note)
	checkErr(err, "Could not write note chunk")

	offset, err := output.Seek(0, io.SeekCurrent)
	checkErr(err, "Could not check file size")

	_, err = output.Seek(0, io.SeekStart)
	checkErr(err, "Could not seek to start")

	header.ChunkSize = int32(offset)
	err = binary.Write(output, binary.LittleEndian, &header)
	checkErr(err, "Could not update header")
}
