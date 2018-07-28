package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"time"
)

const (
	// chSpace is the utf8 space character
	chSpace = 0x20

	// Maximum lookback offset we can pack in the 2-byte struct
	lz77WindowSize = 0x7FF

	lz77MaxChunkLen = 10
	lz77MinChunkLen = 3
)

func printStruct(x interface{}) {
	ref := reflect.ValueOf(x)

	if ref.Kind() == reflect.Ptr {
		ref = ref.Elem()
	}

	var CurPos uintptr = 0
	fmt.Println("---------------------- " + ref.Type().Name() + " ----------------------")
	for i := 0; i < ref.NumField(); i++ {
		val := ref.Field(i)
		typ := ref.Type().Field(i)
		//: %-10v , int(CurPos)+int(typ.Type.Size())

		var value interface{}
		switch typ.Tag.Get("format") {
		case "bits":
			value = fmt.Sprintf("bit(%b)", val.Interface())
		case "string":
			value = fmt.Sprintf("%s", val.Interface())
		case "hex":
			value = fmt.Sprintf("% x", val.Interface())
		case "date":
			if tim_, err := strconv.ParseInt(val.String(), 10, 64); err != nil {
				//BUG(fix): Check Mac/Unix timestamp format
				//If the time has the top bit set, it's an unsigned 32-bit number counting from 1st Jan 1904
				//If the time has the top bit clear, it's a signed 32-bit number counting from 1st Jan 1970.
				value = time.Unix(tim_, 0)
			} else {
				value = val.Interface()
			}
		default:
			value = val.Interface()
		}

		fmt.Println(fmt.Sprintf("%-25v", typ.Name), fmt.Sprintf("%-5v:", CurPos), value)
		CurPos += typ.Type.Size()
		//}

	}
}

func hasBit(n int, pos uint) bool {
	val := n & (1 << pos)
	return (val > 0)
}

func getExthMetaByTag(tag uint32) mobiExthMeta {
	for i := 0; i < len(ExthMeta); i++ {
		if ExthMeta[i].ID == tag {
			return ExthMeta[i]
		}
	}
	return ExthMeta[0]
}

var setBits [256]uint8 = [256]uint8{
	0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4,
	1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5,
	1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7,
	1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7,
	2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6,
	3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7,
	3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7,
	4, 5, 5, 6, 5, 6, 6, 7, 5, 6, 6, 7, 6, 7, 7, 8,
}

// VwiDec decoders variable lenght integer. Returns value and number of bytes consumed
func vwiDec(src []uint8, forward bool) (uint32, uint32) {
	var val uint32   //val = 0
	var byts []uint8 // byts = bytearray()

	if !forward { //if not forward:
		for i, j := 0, len(src)-1; i < j; i, j = i+1, j-1 { //     src.reverse()
			src[i], src[j] = src[j], src[i]
		}
	}
	for _, bnum := range src {
		mask := ^(uint8(1) << 7)
		byts = append(byts, bnum&mask)
		if bnum>>7 == 1 {
			break
		}
	}

	if !forward { //if not forward:
		for i, j := 0, len(byts)-1; i < j; i, j = i+1, j-1 { //     src.reverse()
			byts[i], byts[j] = byts[j], byts[i]
		}
	}

	for _, Byte := range byts {
		val = val << 7
		val |= uint32(Byte)
	}

	return val, uint32(len(byts))
}

func vwiEncInt(x int) []uint8 {
	buf := make([]uint8, 64)
	z := 0
	for {
		buf[z] = byte(x) & 0x7f
		x >>= 7
		z++
		if x == 0 {
			break
		}
	}
	buf[0] |= 0x80
	for i, j := 0, z-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return buf[:z]
}

func minimizeHTML(x []byte) []byte {
	//Clear multiple spaces
	out := regexp.MustCompile("[ ]+").ReplaceAllString(string(x), " ")
	out = regexp.MustCompile("[\t\r\n]").ReplaceAllString(out, "")
	//Clear tabs, new lines
	return []byte(out)
}

var mask_to_bit_shifts = map[int]uint8{1: 0, 2: 1, 3: 0, 4: 2, 8: 3, 12: 2, 16: 4, 32: 5, 48: 4, 64: 6, 128: 7, 192: 6}

func controlByte(tagx []mobiTagxTags) []byte {
	var cbs []byte
	var ans uint8 = 0
	for _, tags := range tagx {
		if tags.Control_Byte == 1 {
			cbs = append(cbs, ans)
			ans = 0
			continue
		}
		nvals := uint8(1)
		nentries := nvals / tags.TagNum
		shifts := mask_to_bit_shifts[int(tags.Bitmask)]
		ans |= tags.Bitmask & (nentries << shifts)
	}
	return cbs
}

func stringToBytes(value string, output interface{}) {
	out := reflect.ValueOf(output).Elem()

	for i := 0; i < out.Type().Len(); i++ {
		if i > len(value)-1 {
			break
		}
		out.Index(i).Set(reflect.ValueOf(byte(value[i])))
	}
}

func underlineTitle(x string) string {
	x = regexp.MustCompile("[^-A-Za-z0-9]").ReplaceAllString(x, "_")
	if len(x) > 31 {
		return x[:31]
	}
	return x
}

func palmDocLZ77Pack(data []byte) []byte {
	var outB []byte

	var tailLen = int(data[len(data)-1])
	var tail = data[(len(data)-1)-tailLen:] /*-multibyte*/
	data = data[:(len(data)-1)-tailLen]     /* -multibyte*/

	var ldata = len(data)

	for i := 0; i < ldata; i++ {
		if i > lz77MaxChunkLen && (ldata-i) > lz77MaxChunkLen {

			// Only seek through the available window size
			boundOffset := i - lz77WindowSize
			if boundOffset < 0 {
				boundOffset = 0
			}

			// Lookup the largest chunk we can find in the window
			relIdx, chunkLen := lz77Lookup(data[boundOffset:i], data[i:i+lz77MaxChunkLen])

			// Encode the chunk if we found one
			if relIdx > -1 {
				absIdx := boundOffset + relIdx // get an absolute index
				offset := int64(i) - int64(absIdx)
				// pack it into the data structure
				code := 0x8000 + (offset << 3) + (int64(chunkLen) - lz77MinChunkLen)
				// Write the two-byte structure
				outB = append(outB, byte(code>>8))
				outB = append(outB, byte(code))
				i += chunkLen - 1 // jump forward, taking the i++ into account
				continue          // Skip to next chunk to encode
			}
		}

		// We did not find any chunks we could use
		och := data[i]

		// Pack space characters and following characters together
		if och == chSpace && (i+1) < ldata {
			onch := data[i+1]
			if onch >= 0x40 && onch < 0x80 {
				// if following is an ascii letter
				// indicate space by setting 8th bit
				outB = append(outB, onch^0x80)
				i++
				continue
			} else {
				// just output the space
				outB = append(outB, och)
				continue
			}
		}

		if och == 0 || (och > 8 && och < 0x80) {
			// A single-byte character. Just append it
			outB = append(outB, och)
		} else {
			// Multi-byte character - get all of them appended in
			j := i
			var binseq []byte
			for {
				if j < ldata && len(binseq) < 8 {
					och = data[j]
					if och == 0 || (och > 8 && och < 0x80) {
						break
					}
					binseq = append(binseq, och)
					j++
				} else {
					break
				}
			}
			outB = append(outB, byte(len(binseq)))
			outB = append(outB, binseq...)

			i += len(binseq) - 1
		}
	}
	outB = append(outB, tail...)
	return outB
}

// lz77Lookup implements a single-pass lookup for all lengths of chunks.
// It returns the longest chunk it found and the last index of that chunk.
// It returns -1, _ if the chunk could not be found at all
func lz77Lookup(window, chunk []byte) (idx, foundLen int) {
	chLen := len(chunk)
	if chLen < lz77MinChunkLen {
		panic("Unable to search for chunks smaller than min chunk size!")
	}
	if chLen > lz77MaxChunkLen {
		panic("Unable to search for chunks larger than the max chunk size!")
	}

	idxs := make([]int, chLen)
	for i := range idxs {
		idxs[i] = -1
	}

	foundLen = lz77MinChunkLen - 1 // haven't found any, but guaranteed to be > 0

	c := chunk[0]

	// Running backwards through the window to find the last indices
	// stop if we've found a chunk of the full chunk length
	for idx := len(window) - lz77MinChunkLen; foundLen < chLen && idx >= 0; idx-- {
		if window[idx] == c { // shortcut check before expensive operation
			// Ignoring the length of chunk we've already found, try all longer chunk lenghts
			for currLen := foundLen + 1; currLen <= chLen; currLen++ {
				eq := bytes.Equal(window[idx:idx+currLen], chunk[:currLen])
				if eq {
					idxs[currLen-1] = idx
					// We've found the current length, only search for longer chunks now
					foundLen = currLen
				} else {
					// Longer chunks have this one as prefix, and will not be present
					break
				}
			}
		}
	}

	return idxs[foundLen-1], foundLen
}

func int32ToBytes(i uint32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, i)
	return buf.Bytes()
}
