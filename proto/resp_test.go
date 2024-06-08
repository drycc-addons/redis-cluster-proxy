package proto

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"testing"
)

var (
	respSimpleString     = Data{T: T_SimpleString, String: []byte("OK")}
	respSimpleStringText = "+OK\r\n"

	respError     = Data{T: T_Error, String: []byte("Error message")}
	respErrorText = "-Error message\r\n"

	respBulkString     = Data{T: T_BulkString, String: []byte("foobar")}
	respBulkStringText = "$6\r\nfoobar\r\n"

	respNilBulkString     = Data{T: T_BulkString, IsNil: true}
	respNilBulkStringText = "$-1\r\n"

	respInteger     = Data{T: T_Integer, Integer: 1000}
	respIntegerText = ":1000\r\n"

	respArray     = Data{T: T_Array, Array: []*Data{&respSimpleString, &respInteger}}
	respArrayText = "*2\r\n" + respSimpleStringText + respIntegerText
)

var validCommand map[string]string
var validData map[string]Data

func TestValidData(t *testing.T) {
	for text, data := range validData {
		buf := bufio.NewReader(bytes.NewReader([]byte(text)))
		//test read
		d, err := ReadData(buf)
		if nil != err || d.T != data.T {
			t.Error(err, text, data.T)
		}

		if false == eqData(*d, data) {
			t.Error(text, *d, data)
		}

		//test format
		if text != string(data.Format()) {
			t.Error(text, data)
		}
	}
}

func BenchmarkDataFormat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, data := range validData {
			data.Format()
		}
	}
}

func BenchmarkReadData(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for text := range validData {
			b.StopTimer()
			buf := bufio.NewReader(bytes.NewReader([]byte(text)))
			b.StartTimer()
			ReadData(buf)
		}
	}
}

func BenchmarkReadDataBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for text := range validData {
			b.StopTimer()
			buf := bufio.NewReader(bytes.NewReader([]byte(text)))
			b.StartTimer()
			o := NewObject()
			ReadDataBytes(buf, o)
		}
	}
}
func eqData(d1, d2 Data) bool {
	eqType := d1.T == d2.T
	eqString := 0 == bytes.Compare(d1.String, d2.String)
	eqInteger := d1.Integer == d2.Integer
	eqNil := d1.IsNil == d2.IsNil
	eqArrayLen := len(d1.Array) == len(d2.Array)
	eqArray := true
	if len(d1.Array) > 0 && eqArrayLen {
		for index := range d1.Array {
			if false == eqData(*d1.Array[index], *d2.Array[index]) {
				eqArray = false
				break
			}
		}
	}
	return eqType && eqString && eqInteger && eqNil && eqArrayLen && eqArray
}

func TestValidCommand(t *testing.T) {
	for input, cmd := range validCommand {
		reader := bufio.NewReader(bytes.NewReader([]byte(input)))
		c, err := ReadCommand(reader)
		if nil != err {
			t.Error("read command error", err)
		} else if c.Name() != cmd {
			t.Error("read command error", c.Name(), cmd)
		}
	}
}

func TestWriteRead(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	count := 1000
	for i := 0; i < count; i++ {
		cmd, err := NewCommand("SET", fmt.Sprintf("%d", i), fmt.Sprintf("%d", i))
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := w.Write(cmd.Format()); err != nil {
			t.Error(err)
			return
		}
	}
	if err := w.Flush(); err != nil {
		t.Error(err)
		return
	}

	r := bufio.NewReader(&b)
	for i := 0; i < count; i++ {
		_, err := ReadCommand(r)
		if err != nil {
			t.Error(err)
			return
		}
	}
	if _, err := ReadCommand(r); err != io.EOF {
		t.Error(err)
	}
}

func TestReadDataBytes(t *testing.T) {
	cases := []string{
		"-MOVED 135 127.0.0.1:7003\r\n",
		"*2\r\n$3\r\nget\r\n$3\r\naaa\r\n",
		"$3\r\nbbb\r\n",
	}
	for _, cc := range cases {
		r := bufio.NewReader(bytes.NewBufferString(cc))
		o := NewObject()
		if err := ReadDataBytes(r, o); err != nil {
			t.Error(err)
		}
		if !bytes.Equal(o.Raw(), []byte(cc)) {
			t.Errorf("expected: %s, got: %s", cc, o.Raw())
		}
		if _, err := r.Peek(1); err != io.EOF {
			t.Errorf("expected EOFs, got: %s", err)
		}
	}
}

func TestReadCommand(t *testing.T) {
	r := bufio.NewReader(bytes.NewBufferString("\r\n"))
	if _, err := ReadCommand(r); err != nil {
		t.Log("OK")
	}
}

func _validCommand(b *testing.B) {
	for input, cmd := range validCommand {
		b.StopTimer()
		reader := bufio.NewReader(bytes.NewReader([]byte(input)))
		b.StartTimer()
		c, err := ReadCommand(reader)
		if nil != err {
			b.Error("read command error", err)
		} else if c.Name() != cmd {
			b.Error("read command error", c.Name(), cmd)
		}
	}
}

func BenchmarkValidCommand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_validCommand(b)
	}
}

func BenchmarkCommandFormat(b *testing.B) {
	cmd, _ := NewCommand("LLEN", "walu.cc")
	for i := 0; i < b.N; i++ {
		cmd.Format()
	}
}

func init() {
	validCommand = map[string]string{
		"PING\r\n":                             "PING",
		"EXISTS foo\r\n":                       "EXISTS",
		"*2\r\n$4\r\nLLEN\r\n$6\r\nmysist\r\n": "LLEN",
	}

	validData = map[string]Data{
		respSimpleStringText:  respSimpleString,
		respErrorText:         respError,
		respBulkStringText:    respBulkString,
		respNilBulkStringText: respNilBulkString,
		respIntegerText:       respInteger,
		respArrayText:         respArray,
	}
}
