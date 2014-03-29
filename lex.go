package main

import (
	"flag"
	"fmt"
	"strings"
	"unicode/utf8"
)

/*
Personal test of lexical scanning
Talk:		https://www.youtube.com/watch?v=HxaD_trXwRE
Slides:		http://cuddle.googlecode.com/hg/talk/lex.html
Example:	http://golang.org/src/pkg/text/template/parse/lex.go
*/

// item thrown over the fence
type item struct {
	typ itemType
	val string
}

type itemType int

const (
	itemError itemType = iota
	itemEOF
	itemLeftMeta
	itemRightMeta
	itemNumber
	itemText
)

func (i item) String() string {
	switch i.typ {
	case itemEOF:
		return "EOF"
	case itemError:
		return i.val
	}
	// truncating
	if len(i.val) > 10 {
		return fmt.Sprintf("%.10q...", i.val) // safety escaped
	}
	return fmt.Sprintf("%q", i.val)
}

// state function returns next state (function)
type stateFn func(*lexer) stateFn

type lexer struct {
	name  string    // arbitrary name
	input string    // input
	state stateFn   // next state
	start int       // last start position (current item start)
	pos   int       // current position in input
	width int       // last rune size
	items chan item // items over the fence
}

// emit throws items over the fence (to client)
func (l *lexer) emit(t itemType) {
	l.items <- item{t, l.input[l.start:l.pos]}
	l.start = l.pos
}

const (
	leftMeta  = "{{"
	rightMeta = "}}"
)

const eof = -1

// states:
// plaintext
func lexText(l *lexer) stateFn {
	// scan until {{ is found
	for {
		if strings.HasPrefix(l.input[l.pos:], leftMeta) {
			// check if we have un-emitted (buffer) plaintext
			if l.pos > l.start {
				l.emit(itemText)
			}
			// change state to left meta
			return lexLeftMeta
		}
		if l.next() == eof {
			break
		}
	}
	// eof, check if we have buffered plaintext
	if l.pos > l.start {
		l.emit(itemText)
	}
	// let the client know we're done
	l.emit(itemEOF)
	// terminate the state machine
	return nil
}

// metas
func lexLeftMeta(l *lexer) stateFn {
	l.pos += len(leftMeta)
	l.emit(itemLeftMeta)
	// change state to insideBlock
	return lexInsideBlock
}

func lexRightMeta(l *lexer) stateFn {
	l.pos += len(rightMeta)
	l.emit(itemRightMeta)
	return lexText
}

// block
func lexInsideBlock(l *lexer) stateFn {
	// scan until }} is found
	for {
		if strings.HasPrefix(l.input[l.pos:], rightMeta) {
			// QUESTION: why not checking buffering?
			return lexRightMeta
		}
		switch r := l.next(); {
		case r == eof || r == '\n':
			return l.errorf("unclosed block")
		case r == ' ' || r == '\t':
			l.ignore()
		case r == '+' || r == '-' || r >= '0' && r <= '9':
			l.backup()
			return lexNumber
		default:
			return l.errorf("unexpected char in block: %#U", r)
		}
	}
}

// numbers
const digits = "0123456789"

func lexNumber(l *lexer) stateFn {
	l.accept("+-")
	l.acceptRun(digits)
	l.emit(itemNumber)
	return lexInsideBlock
}

// helpers
func (l *lexer) ignore() {
	l.start = l.pos
}

func (l *lexer) backup() {
	l.pos -= l.width
}

// get next rune, but rewind (backup)
func (l *lexer) peak() rune {
	r := l.next()
	l.backup()
	return r
}

// check if next rune is in valid range
func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	// otherwise rewind and deny
	l.backup()
	return false
}

// read until no longer valid
func (l *lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	// unwanted rune reached, rewind
	l.backup()
}

func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	// throw error over the fence
	l.items <- item{
		itemError,
		fmt.Sprintf(format, args),
	}
	// abort state machine
	return nil
}

// API for traversing and/or parsing

// return new scanner
func NewScanner(name, input string) *lexer {
	return &lexer{
		name:  name,
		input: input,
		state: lexText,
		items: make(chan item, 2), // might not be needed here, but no reason to let memory go above what's needed
	}
}
func (l *lexer) next() (r rune) {
	// check if end has been reached
	if l.pos >= len(l.input) {
		l.width = 0 // QUESTION: to not break backup?
		return eof
	}
	// read next rune
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += l.width
	return r
}

// state machine
func (l *lexer) nextItem() item {
	for {
		select {
		case i := <-l.items:
			return i
		default:
			l.state = l.state(l)
		}
	}
	// we've escaped the state functions
	panic("no state function, but still in state machine")
}

func main() {
	flag.Parse()
	input := flag.Arg(0)
	fmt.Printf("lexing %.100q...\n", input)
	s := NewScanner("number lexer", input)
	for {
		if i := s.nextItem(); i.typ != itemEOF {
			fmt.Println(i)
		} else {
			break
		} 
	}
}
