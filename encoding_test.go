package zip

import (
	"bytes"
	"encoding/binary"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

func TestReadDirectoryHeaderDecodesNameAndCommentEncodings(t *testing.T) {
	tests := []struct {
		name            string
		flags           uint16
		fileName        string
		comment         string
		nameEncoding    string
		commentEncoding string
		enc             encoding.Encoding
	}{
		{
			name:            "UTF8Flag",
			flags:           utf8Flag,
			fileName:        "3 希望实验一次成/文件.txt",
			comment:         "中文注释",
			nameEncoding:    "utf-8",
			commentEncoding: "utf-8",
		},
		{
			name:            "CP437",
			fileName:        "Dance_QOS.zip",
			comment:         "plain comment",
			nameEncoding:    "utf-8",
			commentEncoding: "utf-8",
			enc:             charmap.CodePage437,
		},
		{
			name:            "GB18030",
			fileName:        "3 希望实验一次成/文件.txt",
			comment:         "中文注释",
			nameEncoding:    "gb18030",
			commentEncoding: "gb18030",
			enc:             simplifiedchinese.GB18030,
		},
		{
			name:            "Big5",
			fileName:        "3 希望實驗一次成/檔案.txt",
			comment:         "繁體註釋",
			nameEncoding:    "big5",
			commentEncoding: "big5",
			enc:             traditionalchinese.Big5,
		},
		{
			name:            "ShiftJIS",
			fileName:        "希望テスト/ファイル.txt",
			comment:         "日本語コメント",
			nameEncoding:    "shift-jis",
			commentEncoding: "shift-jis",
			enc:             japanese.ShiftJIS,
		},
		{
			name:            "EUCKR",
			fileName:        "희망실험/파일.txt",
			comment:         "한국어주석",
			nameEncoding:    "euc-kr",
			commentEncoding: "euc-kr",
			enc:             korean.EUCKR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nameBytes := encodeForTest(t, tt.fileName, tt.enc, tt.flags)
			commentBytes := encodeForTest(t, tt.comment, tt.enc, tt.flags)
			header := buildDirectoryHeader(nameBytes, commentBytes, tt.flags)
			f := &File{}
			if err := readDirectoryHeader(f, bytes.NewReader(header)); err != nil {
				t.Fatalf("readDirectoryHeader() error = %v", err)
			}
			if f.Name != tt.fileName {
				t.Fatalf("Name = %q, want %q", f.Name, tt.fileName)
			}
			if f.Comment != tt.comment {
				t.Fatalf("Comment = %q, want %q", f.Comment, tt.comment)
			}
			if f.NameEncoding != tt.nameEncoding {
				t.Fatalf("NameEncoding = %q, want %q", f.NameEncoding, tt.nameEncoding)
			}
			if f.CommentEncoding != tt.commentEncoding {
				t.Fatalf("CommentEncoding = %q, want %q", f.CommentEncoding, tt.commentEncoding)
			}
		})
	}
}

func TestReadDirectoryHeaderPrefersGB18030OverMojibake(t *testing.T) {
	fileName := "3 希望实验一次成/Dance_QOS.z01"
	rawName := encodeForTest(t, fileName, simplifiedchinese.GB18030, 0)
	header := buildDirectoryHeader(rawName, nil, 0)
	f := &File{}
	if err := readDirectoryHeader(f, bytes.NewReader(header)); err != nil {
		t.Fatalf("readDirectoryHeader() error = %v", err)
	}
	if f.Name != fileName {
		t.Fatalf("Name = %q, want %q", f.Name, fileName)
	}
	if f.NameEncoding != "gb18030" {
		t.Fatalf("NameEncoding = %q, want gb18030", f.NameEncoding)
	}
}

func buildDirectoryHeader(nameBytes, commentBytes []byte, flags uint16) []byte {
	buf := new(bytes.Buffer)
	writeUint32(buf, directoryHeaderSignature)
	writeUint16(buf, zipVersion20)
	writeUint16(buf, zipVersion20)
	writeUint16(buf, flags)
	writeUint16(buf, Store)
	writeUint16(buf, 0)
	writeUint16(buf, 0)
	writeUint32(buf, 0)
	writeUint32(buf, 0)
	writeUint32(buf, 0)
	writeUint16(buf, uint16(len(nameBytes)))
	writeUint16(buf, 0)
	writeUint16(buf, uint16(len(commentBytes)))
	writeUint16(buf, 0)
	writeUint16(buf, 0)
	writeUint32(buf, 0)
	writeUint32(buf, 0)
	_, _ = buf.Write(nameBytes)
	_, _ = buf.Write(commentBytes)
	return buf.Bytes()
}

func encodeForTest(t *testing.T, value string, enc encoding.Encoding, flags uint16) []byte {
	t.Helper()
	if flags&utf8Flag != 0 || enc == nil {
		return []byte(value)
	}
	out, _, err := transform.Bytes(enc.NewEncoder(), []byte(value))
	if err != nil {
		t.Fatalf("encode %q failed: %v", value, err)
	}
	return out
}

func writeUint16(buf *bytes.Buffer, value uint16) {
	_ = binary.Write(buf, binary.LittleEndian, value)
}

func writeUint32(buf *bytes.Buffer, value uint32) {
	_ = binary.Write(buf, binary.LittleEndian, value)
}
