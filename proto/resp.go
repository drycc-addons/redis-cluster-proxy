// a simple and high-performance library for parsing and encoding valkey protocal and valkey inline command
package proto

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
)

const (
	T_SimpleString = '+'
	T_Error        = '-'
	T_Integer      = ':'
	T_BulkString   = '$'
	T_Array        = '*'
)

var (
	CRLF        = []byte{'\r', '\n'}
	errProtocol = errors.New("protocol error")
)

/*
Command

valkey supports two kinds of Command: (Inline Command) and (Array With BulkString)
*/
type Command struct {
	Args []string //Args[0] is the command name
}

// get the command name
func (c Command) Name() string {
	if len(c.Args) == 0 {
		return ""
	} else {
		return c.Args[0]
	}
}

// get command.Args[index] in string
//
// I must change the method name from String to Value, because method named String has specical meaning when working with fmt.Sprintf.
func (c Command) Value(index int) (ret string) {
	if len(c.Args) > index {
		ret = c.Args[index]
	}
	return ret
}

// get command.Args[index] in int64.
// return 0 if it isn't numberic string.
func (c Command) Integer(index int) (ret int64) {
	if len(c.Args) > index {
		ret, _ = strconv.ParseInt(c.Args[index], 10, 64)
	}
	return ret
}

// Foramat a command into ArrayWithBulkString
func (c Command) Format() []byte {
	ret := new(bytes.Buffer)

	ret.WriteByte(T_Array)
	ret.WriteString(strconv.Itoa(len(c.Args)))
	ret.Write(CRLF)
	for index := range c.Args {
		ret.WriteByte(T_BulkString)
		ret.WriteString(strconv.Itoa(len(c.Args[index])))
		ret.Write(CRLF)
		ret.WriteString(c.Args[index])
		ret.Write(CRLF)
	}
	return ret.Bytes()
}

/*
make a new command like terminal

	cmd, err := NewCommand("get", "username")
*/
func NewCommand(args ...string) (*Command, error) {
	if len(args) == 0 {
		return nil, errors.New("err_new_cmd")
	}
	return &Command{Args: args}, nil
}

// read a command from bufio.Reader
func ReadCommand(r *bufio.Reader) (*Command, error) {
	buf, err := readRespCommandLine(r)
	if nil != err && !(io.EOF == err && len(buf) > 1) {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, errProtocol
	}
	if T_Array != buf[0] {
		return NewCommand(strings.Fields(strings.TrimSpace(string(buf)))...)
	}

	//Command: BulkString
	var ret *Data
	ret = new(Data)

	ret, err = readDataForSpecType(r, buf)
	if nil != err {
		return nil, err
	}

	commandArgs := make([]string, len(ret.Array))
	for index := range ret.Array {
		if ret.Array[index].T != T_BulkString {
			return nil, errors.New("unexpected Command Type")
		}
		commandArgs[index] = string(ret.Array[index].String)
	}

	return NewCommand(commandArgs...)
}

// a resp package
type Data struct {
	T       byte
	String  []byte
	Integer int64
	Array   []*Data
	IsNil   bool
}

// format Data into resp string
func (d Data) Format() []byte {
	ret := new(bytes.Buffer)

	ret.WriteByte(d.T)
	if d.IsNil {
		ret.WriteString("-1")
		ret.Write(CRLF)
		return ret.Bytes()
	}

	switch d.T {
	case T_SimpleString, T_Error:
		ret.Write(d.String)
		ret.Write(CRLF)
	case T_BulkString:
		ret.WriteString(strconv.Itoa(len(d.String)))
		ret.Write(CRLF)
		ret.Write(d.String)
		ret.Write(CRLF)
	case T_Integer:
		ret.WriteString(strconv.FormatInt(d.Integer, 10))
		ret.Write(CRLF)
	case T_Array:
		ret.WriteString(strconv.Itoa(len(d.Array)))
		ret.Write(CRLF)
		for index := range d.Array {
			ret.Write(d.Array[index].Format())
		}
	}
	return ret.Bytes()
}

// get a data from bufio.Reader
func ReadData(r *bufio.Reader) (*Data, error) {
	buf, err := readRespLine(r)
	if nil != err {
		return nil, err
	}

	if len(buf) < 2 {
		return nil, errors.New("invalid Data Source: " + string(buf))
	}

	return readDataForSpecType(r, buf)
}

func readDataForSpecType(r *bufio.Reader, line []byte) (*Data, error) {

	var err error

	ret := new(Data)
	switch line[0] {
	case T_SimpleString:
		ret.T = T_SimpleString
		ret.String = line[1:]

	case T_Error:
		ret.T = T_Error
		ret.String = line[1:]

	case T_Integer:
		ret.T = T_Integer
		ret.Integer, err = strconv.ParseInt(string(line[1:]), 10, 64)

	case T_BulkString:
		var lenBulkString int64
		lenBulkString, err = strconv.ParseInt(string(line[1:]), 10, 64)
		ret.T = T_BulkString
		if lenBulkString != -1 {
			data := make([]byte, lenBulkString+2)
			readRespN(r, &data)
			ret.String = data[:lenBulkString]
		} else {
			ret.IsNil = true
		}

	case T_Array:
		var lenArray int64
		var i int64
		lenArray, err = strconv.ParseInt(string(line[1:]), 10, 64)

		ret.T = T_Array
		if nil == err {
			if lenArray != -1 {
				ret.Array = make([]*Data, lenArray)
				for i = 0; i < lenArray && nil == err; i++ {
					ret.Array[i], err = ReadData(r)
				}
			} else {
				ret.IsNil = true
			}
		}

	default: //Maybe you are Inline Command
		err = errors.New("unexpected type ")

	}
	return ret, err
}

func readDataBytesForSpecType(r *bufio.Reader, line []byte, obj *Object) error {
	switch line[0] {
	case T_SimpleString, T_Error, T_Integer:
		return nil
	case T_BulkString:
		lenBulkString, err := strconv.ParseInt(string(line[1:]), 10, 64)
		if err != nil {
			return err
		}
		if lenBulkString != -1 {
			buf := make([]byte, lenBulkString+2)
			err := readRespN(r, &buf)
			if err != nil {
				return err
			} else {
				obj.Append(buf)
			}
		}
		// else if nil

	case T_Array:
		lenArray, err := strconv.ParseInt(string(line[1:]), 10, 64)
		if err != nil {
			return err
		}
		var i int64
		if lenArray != -1 {
			for i = 0; i < lenArray; i++ {
				if err := ReadDataBytes(r, obj); err != nil {
					return err
				}
			}
		}
		// else is nil

	default:
		return errors.New("unexpected type ")
	}
	return nil
}

// read a resp line and trim the last \r\n
func readRespLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if n := len(line); n < 2 {
		return nil, errProtocol
	} else {
		return line[:n-2], nil
	}
}

func readRespLineBytes(r *bufio.Reader, obj *Object) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if n := len(line); n < 2 {
		return nil, errProtocol
	} else {
		obj.Append(line)
		return line[:n-2], nil
	}
}

// read a valkey InlineCommand
func readRespCommandLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if n := len(line); n < 2 {
		return nil, errProtocol
	} else {
		return line[:n-2], nil
	}
}

// read the next N bytes
func readRespN(r *bufio.Reader, data *[]byte) error {
	if _, err := io.ReadFull(r, *data); err != nil {
		return err
	} else {
		return nil
	}
}

type Object struct {
	raw bytes.Buffer
}

func NewObject() *Object {
	o := &Object{}
	o.raw.Grow(1024 * 256)
	return o
}

func NewObjectFromData(data *Data) *Object {
	o := &Object{}
	o.Append(data.Format())
	return o
}

func (o *Object) Append(buf []byte) {
	o.raw.Write(buf)
}

func (o *Object) Raw() []byte {
	return o.raw.Bytes()
}

// read data bytes reads a full RESP object bytes
func ReadDataBytes(r *bufio.Reader, obj *Object) error {
	buf, err := readRespLineBytes(r, obj)
	if err != nil {
		return err
	}

	if len(buf) < 2 {
		return errors.New("invalid Data Source: " + string(buf))
	}

	return readDataBytesForSpecType(r, buf, obj)
}
