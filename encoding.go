package zip

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

const utf8Flag = 1 << 11

type candidateEncoding struct {
	name string
	enc  encoding.Encoding
}

var candidateEncodings = []candidateEncoding{
	{name: "utf-8"},
	{name: "cp437", enc: charmap.CodePage437},
	{name: "gb18030", enc: simplifiedchinese.GB18030},
	{name: "big5", enc: traditionalchinese.Big5},
	{name: "shift-jis", enc: japanese.ShiftJIS},
	{name: "euc-kr", enc: korean.EUCKR},
}

func decodeName(raw []byte, flags uint16) (string, string) {
	if flags&utf8Flag != 0 && utf8.Valid(raw) {
		return string(raw), "utf-8"
	}
	if isASCII(raw) {
		return string(raw), "utf-8"
	}
	bestName := string(raw)
	bestEnc := "binary"
	bestScore := -1 << 30
	for _, candidate := range candidateEncodings {
		decoded, ok := decodeBytes(raw, candidate)
		if !ok {
			continue
		}
		score := scoreDecodedPath(decoded, raw, candidate, flags)
		if score > bestScore || score == bestScore && preferCandidate(candidate, bestEnc, flags) {
			bestScore = score
			bestName = decoded
			bestEnc = candidate.name
		}
	}
	return bestName, bestEnc
}

func decodeComment(raw []byte, flags uint16) (string, string) {
	return decodeCommentWithPreferred(raw, flags, "")
}

func decodeCommentWithPreferred(raw []byte, flags uint16, preferred string) (string, string) {
	if flags&utf8Flag != 0 && utf8.Valid(raw) {
		return string(raw), "utf-8"
	}
	if isASCII(raw) {
		return string(raw), "utf-8"
	}
	bestComment := string(raw)
	bestEnc := "binary"
	bestScore := -1 << 30
	for _, candidate := range candidateEncodings {
		decoded, ok := decodeBytes(raw, candidate)
		if !ok {
			continue
		}
		score := scoreDecodedText(decoded, raw, candidate, flags)
		if candidate.name == preferred {
			score += 60
		}
		if score > bestScore || score == bestScore && preferCandidate(candidate, bestEnc, flags) {
			bestScore = score
			bestComment = decoded
			bestEnc = candidate.name
		}
	}
	return bestComment, bestEnc
}

func decodeBytes(raw []byte, candidate candidateEncoding) (string, bool) {
	if candidate.name == "utf-8" {
		if !utf8.Valid(raw) {
			return "", false
		}
		return string(raw), true
	}
	decoded, _, err := transform.Bytes(candidate.enc.NewDecoder(), raw)
	if err != nil {
		return "", false
	}
	return string(decoded), true
}

func scoreDecodedPath(decoded string, raw []byte, candidate candidateEncoding, flags uint16) int {
	score := baseScore(decoded, raw, candidate, flags)
	if score < -1000 {
		return score
	}
	separators := strings.Count(decoded, "/") + strings.Count(decoded, "\\")
	score += separators * 4
	score += pathCharScore(decoded)
	score += scriptRunScore(decoded)
	score += candidateScriptBonus(decoded, candidate)
	score -= mojibakePenalty(decoded)
	return score
}

func scoreDecodedText(decoded string, raw []byte, candidate candidateEncoding, flags uint16) int {
	score := baseScore(decoded, raw, candidate, flags)
	if score < -1000 {
		return score
	}
	score += printableScore(decoded)
	score += scriptRunScore(decoded)
	score += candidateScriptBonus(decoded, candidate)
	score -= mojibakePenalty(decoded) / 2
	return score
}

func baseScore(decoded string, raw []byte, candidate candidateEncoding, flags uint16) int {
	if decoded == "" && len(raw) != 0 {
		return -1 << 20
	}
	score := 0
	if candidate.name == "utf-8" && flags&utf8Flag != 0 && utf8.Valid(raw) {
		score += 100
	}
	if candidate.name == "utf-8" && utf8.Valid(raw) {
		score += 40
	}
	if !isRoundTrip(decoded, raw, candidate) {
		return -1 << 20
	}
	score += 30
	fffdCount := strings.Count(decoded, "\uFFFD")
	if fffdCount > 0 {
		score -= 100 * fffdCount
	}
	if hasControl(decoded) {
		score -= 80
	}
	score += printableScore(decoded)
	return score
}

func isRoundTrip(decoded string, raw []byte, candidate candidateEncoding) bool {
	if candidate.name == "utf-8" {
		return bytes.Equal([]byte(decoded), raw)
	}
	encoded, _, err := transform.Bytes(candidate.enc.NewEncoder(), []byte(decoded))
	return err == nil && bytes.Equal(encoded, raw)
}

func printableScore(decoded string) int {
	if decoded == "" {
		return 0
	}
	total := 0
	printable := 0
	for _, r := range decoded {
		total++
		if unicode.IsPrint(r) && !unicode.IsControl(r) {
			printable++
		}
	}
	if total == 0 {
		return 0
	}
	return printable * 20 / total
}

func pathCharScore(decoded string) int {
	if decoded == "" {
		return 0
	}
	total := 0
	good := 0
	for _, r := range decoded {
		total++
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			good++
		case strings.ContainsRune(" ._-()[]{}+/\\", r):
			good++
		}
	}
	if total == 0 {
		return 0
	}
	return good * 10 / total
}

func scriptRunScore(decoded string) int {
	bestRun := 0
	current := 0
	for _, r := range decoded {
		if isCJKLike(r) {
			current++
			if current > bestRun {
				bestRun = current
			}
			continue
		}
		current = 0
	}
	if bestRun >= 2 {
		return bestRun * 6
	}
	return 0
}

func candidateScriptBonus(decoded string, candidate candidateEncoding) int {
	han := 0
	hiragana := 0
	katakana := 0
	hangul := 0
	halfwidth := 0
	for _, r := range decoded {
		switch {
		case unicode.In(r, unicode.Han):
			han++
		case unicode.In(r, unicode.Hiragana):
			hiragana++
		case unicode.In(r, unicode.Katakana):
			katakana++
		case unicode.In(r, unicode.Hangul):
			hangul++
		case r >= 0xFF61 && r <= 0xFF9F:
			halfwidth++
		}
	}
	switch candidate.name {
	case "gb18030", "big5":
		return han*6 - halfwidth*8
	case "shift-jis":
		return (hiragana+katakana)*10 + han*2 - halfwidth*2
	case "euc-kr":
		return hangul*12 + han - halfwidth*6
	case "cp437":
		return -(han*6 + hiragana*8 + katakana*8 + hangul*10 + halfwidth*8)
	default:
		return 0
	}
}

func mojibakePenalty(decoded string) int {
	penalty := 0
	greekOrCyrillic := 0
	cjkLike := 0
	for _, r := range decoded {
		switch {
		case unicode.In(r, unicode.Greek, unicode.Cyrillic):
			greekOrCyrillic++
		case isCJKLike(r):
			cjkLike++
		case unicode.IsControl(r):
			penalty += 20
		}
	}
	if greekOrCyrillic > 0 && cjkLike == 0 {
		penalty += greekOrCyrillic * 8
	}
	if strings.Contains(decoded, "ϣ") || strings.Contains(decoded, "��") || strings.Contains(decoded, "�") {
		penalty += 40
	}
	return penalty
}

func hasControl(decoded string) bool {
	for _, r := range decoded {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
}

func isCJKLike(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func preferCandidate(candidate candidateEncoding, currentName string, flags uint16) bool {
	if currentName == "" || currentName == "binary" {
		return true
	}
	order := map[string]int{
		"utf-8":     0,
		"cp437":     1,
		"gb18030":   2,
		"big5":      3,
		"shift-jis": 4,
		"euc-kr":    5,
	}
	if flags&utf8Flag != 0 && candidate.name == "utf-8" {
		return true
	}
	return order[candidate.name] < order[currentName]
}

func isASCII(raw []byte) bool {
	for _, b := range raw {
		if b >= 0x80 {
			return false
		}
	}
	return true
}
