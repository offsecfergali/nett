// Package dns implements nett's native DNS engine: RFC 1035 message
// encode/decode, UDP/TCP/DoH/DoT transports, a caching resolver, and wildcard
// detection. No part of this package shells out to a system resolver binary
// or an external tool — every query is built and parsed by this code.
package dns

import (
	"fmt"
	"strings"
)

// RRType is a DNS resource record type (RFC 1035 §3.2.2 and extensions).
type RRType uint16

const (
	TypeA     RRType = 1
	TypeNS    RRType = 2
	TypeCNAME RRType = 5
	TypeSOA   RRType = 6
	TypePTR   RRType = 12
	TypeMX    RRType = 15
	TypeTXT   RRType = 16
	TypeAAAA  RRType = 28
	TypeSRV   RRType = 33
	TypeCAA   RRType = 257
)

func (t RRType) String() string {
	switch t {
	case TypeA:
		return "A"
	case TypeNS:
		return "NS"
	case TypeCNAME:
		return "CNAME"
	case TypeSOA:
		return "SOA"
	case TypePTR:
		return "PTR"
	case TypeMX:
		return "MX"
	case TypeTXT:
		return "TXT"
	case TypeAAAA:
		return "AAAA"
	case TypeSRV:
		return "SRV"
	case TypeCAA:
		return "CAA"
	default:
		return fmt.Sprintf("TYPE%d", uint16(t))
	}
}

// classIN is the only record class nett ever queries or expects.
const classIN uint16 = 1

// Rcode is a DNS response code (RFC 1035 §4.1.1).
type Rcode int

const (
	RcodeSuccess        Rcode = 0
	RcodeFormatError    Rcode = 1
	RcodeServerFailure  Rcode = 2
	RcodeNameError      Rcode = 3 // NXDOMAIN
	RcodeNotImplemented Rcode = 4
	RcodeRefused        Rcode = 5
)

func (r Rcode) String() string {
	switch r {
	case RcodeSuccess:
		return "NOERROR"
	case RcodeFormatError:
		return "FORMERR"
	case RcodeServerFailure:
		return "SERVFAIL"
	case RcodeNameError:
		return "NXDOMAIN"
	case RcodeNotImplemented:
		return "NOTIMP"
	case RcodeRefused:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", int(r))
	}
}

// Question is one entry in a message's question section.
type Question struct {
	Name string
	Type RRType
}

// MX is the RDATA of an MX record.
type MX struct {
	Preference uint16
	Exchange   string
}

// SRV is the RDATA of an SRV record.
type SRV struct {
	Priority uint16
	Weight   uint16
	Port     uint16
	Target   string
}

// CAA is the RDATA of a CAA record.
type CAA struct {
	Flag  uint8
	Tag   string
	Value string
}

// SOA is the RDATA of an SOA record.
type SOA struct {
	MName   string
	RName   string
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	Minimum uint32
}

// Record is one resource record. Exactly the field matching Type is
// populated; the rest are zero values.
type Record struct {
	Name  string
	Type  RRType
	TTL   uint32
	A     string // dotted-quad
	AAAA  string // colon-hex
	CNAME string
	NS    string
	PTR   string
	MX    *MX
	TXT   []string
	SRV   *SRV
	CAA   *CAA
	SOA   *SOA
}

// Message is a decoded (or to-be-encoded) DNS message.
type Message struct {
	ID                 uint16
	Response           bool
	Opcode             int
	Authoritative      bool
	Truncated          bool
	RecursionDesired   bool
	RecursionAvailable bool
	Rcode              Rcode
	Questions          []Question
	Answers            []Record
	Authority          []Record
	Additional         []Record
}

// NewQuery builds a minimal recursive query for name/qtype. id should be
// randomized per query by the caller (the resolver does this).
func NewQuery(id uint16, name string, qtype RRType) *Message {
	return &Message{
		ID:               id,
		RecursionDesired: true,
		Questions:        []Question{{Name: strings.TrimSuffix(name, "."), Type: qtype}},
	}
}
