package dns

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// header flag bit layout (RFC 1035 §4.1.1), packed into the second uint16 of
// the 12-byte header.
const (
	flagQR = 1 << 15
	flagAA = 1 << 10
	flagTC = 1 << 9
	flagRD = 1 << 8
	flagRA = 1 << 7
)

// Marshal encodes m into RFC 1035 wire format. nett only ever needs to encode
// queries (an empty answer/authority/additional section), but encoding is
// written generally so round-tripping a decoded message is also correct.
func (m *Message) Marshal() ([]byte, error) {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], m.ID)

	var flags uint16
	if m.Response {
		flags |= flagQR
	}
	flags |= uint16(m.Opcode&0xF) << 11
	if m.Authoritative {
		flags |= flagAA
	}
	if m.Truncated {
		flags |= flagTC
	}
	if m.RecursionDesired {
		flags |= flagRD
	}
	if m.RecursionAvailable {
		flags |= flagRA
	}
	flags |= uint16(m.Rcode) & 0xF
	binary.BigEndian.PutUint16(buf[2:4], flags)

	binary.BigEndian.PutUint16(buf[4:6], uint16(len(m.Questions)))
	binary.BigEndian.PutUint16(buf[6:8], uint16(len(m.Answers)))
	binary.BigEndian.PutUint16(buf[8:10], uint16(len(m.Authority)))
	binary.BigEndian.PutUint16(buf[10:12], uint16(len(m.Additional)))

	for _, q := range m.Questions {
		name, err := encodeName(q.Name)
		if err != nil {
			return nil, err
		}
		buf = append(buf, name...)
		buf = appendUint16(buf, uint16(q.Type))
		buf = appendUint16(buf, classIN)
	}
	for _, sec := range [][]Record{m.Answers, m.Authority, m.Additional} {
		for _, rr := range sec {
			b, err := encodeRecord(rr)
			if err != nil {
				return nil, err
			}
			buf = append(buf, b...)
		}
	}
	return buf, nil
}

func appendUint16(buf []byte, v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return append(buf, b[:]...)
}

func appendUint32(buf []byte, v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return append(buf, b[:]...)
}

// encodeName writes name as a sequence of length-prefixed labels terminated
// by a zero-length label. No compression is used on encode: nett-generated
// messages are queries with a single small name, so there is nothing to gain
// and it keeps this direction of the codec simple.
func encodeName(name string) ([]byte, error) {
	name = strings.TrimSuffix(name, ".")
	var buf []byte
	if name == "" {
		return []byte{0}, nil
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("dns: invalid label %q in name %q", label, name)
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0)
	return buf, nil
}

func encodeRecord(rr Record) ([]byte, error) {
	name, err := encodeName(rr.Name)
	if err != nil {
		return nil, err
	}
	buf := append([]byte{}, name...)
	buf = appendUint16(buf, uint16(rr.Type))
	buf = appendUint16(buf, classIN)
	buf = appendUint32(buf, rr.TTL)

	var rdata []byte
	switch rr.Type {
	case TypeA:
		ip := net.ParseIP(rr.A).To4()
		if ip == nil {
			return nil, fmt.Errorf("dns: invalid A address %q", rr.A)
		}
		rdata = ip
	case TypeAAAA:
		ip := net.ParseIP(rr.AAAA).To16()
		if ip == nil {
			return nil, fmt.Errorf("dns: invalid AAAA address %q", rr.AAAA)
		}
		rdata = ip
	case TypeCNAME:
		rdata, err = encodeName(rr.CNAME)
	case TypeNS:
		rdata, err = encodeName(rr.NS)
	case TypePTR:
		rdata, err = encodeName(rr.PTR)
	case TypeMX:
		if rr.MX == nil {
			return nil, fmt.Errorf("dns: MX record missing data")
		}
		rdata = appendUint16(nil, rr.MX.Preference)
		var exch []byte
		exch, err = encodeName(rr.MX.Exchange)
		rdata = append(rdata, exch...)
	case TypeTXT:
		for _, s := range rr.TXT {
			if len(s) > 255 {
				return nil, fmt.Errorf("dns: TXT chunk too long (%d bytes)", len(s))
			}
			rdata = append(rdata, byte(len(s)))
			rdata = append(rdata, s...)
		}
	case TypeSRV:
		if rr.SRV == nil {
			return nil, fmt.Errorf("dns: SRV record missing data")
		}
		rdata = appendUint16(nil, rr.SRV.Priority)
		rdata = appendUint16(rdata, rr.SRV.Weight)
		rdata = appendUint16(rdata, rr.SRV.Port)
		var target []byte
		target, err = encodeName(rr.SRV.Target)
		rdata = append(rdata, target...)
	case TypeCAA:
		if rr.CAA == nil {
			return nil, fmt.Errorf("dns: CAA record missing data")
		}
		rdata = append(rdata, rr.CAA.Flag, byte(len(rr.CAA.Tag)))
		rdata = append(rdata, rr.CAA.Tag...)
		rdata = append(rdata, rr.CAA.Value...)
	case TypeSOA:
		if rr.SOA == nil {
			return nil, fmt.Errorf("dns: SOA record missing data")
		}
		var mname, rname []byte
		mname, err = encodeName(rr.SOA.MName)
		if err == nil {
			rname, err = encodeName(rr.SOA.RName)
		}
		rdata = append(rdata, mname...)
		rdata = append(rdata, rname...)
		rdata = appendUint32(rdata, rr.SOA.Serial)
		rdata = appendUint32(rdata, rr.SOA.Refresh)
		rdata = appendUint32(rdata, rr.SOA.Retry)
		rdata = appendUint32(rdata, rr.SOA.Expire)
		rdata = appendUint32(rdata, rr.SOA.Minimum)
	default:
		return nil, fmt.Errorf("dns: cannot encode record type %s", rr.Type)
	}
	if err != nil {
		return nil, err
	}

	buf = appendUint16(buf, uint16(len(rdata)))
	buf = append(buf, rdata...)
	return buf, nil
}

// Unmarshal decodes an RFC 1035 message from buf.
func Unmarshal(buf []byte) (*Message, error) {
	if len(buf) < 12 {
		return nil, fmt.Errorf("dns: message too short (%d bytes)", len(buf))
	}
	m := &Message{
		ID: binary.BigEndian.Uint16(buf[0:2]),
	}
	flags := binary.BigEndian.Uint16(buf[2:4])
	m.Response = flags&flagQR != 0
	m.Opcode = int((flags >> 11) & 0xF)
	m.Authoritative = flags&flagAA != 0
	m.Truncated = flags&flagTC != 0
	m.RecursionDesired = flags&flagRD != 0
	m.RecursionAvailable = flags&flagRA != 0
	m.Rcode = Rcode(flags & 0xF)

	qdcount := binary.BigEndian.Uint16(buf[4:6])
	ancount := binary.BigEndian.Uint16(buf[6:8])
	nscount := binary.BigEndian.Uint16(buf[8:10])
	arcount := binary.BigEndian.Uint16(buf[10:12])

	off := 12
	var err error
	for i := 0; i < int(qdcount); i++ {
		var q Question
		q.Name, off, err = readName(buf, off)
		if err != nil {
			return nil, err
		}
		if off+4 > len(buf) {
			return nil, fmt.Errorf("dns: truncated question section")
		}
		q.Type = RRType(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 4 // type(2) + class(2)
		m.Questions = append(m.Questions, q)
	}

	sections := make([][]Record, 3)
	for i, n := range []int{int(ancount), int(nscount), int(arcount)} {
		sections[i], off, err = readRecords(buf, off, n)
		if err != nil {
			return nil, err
		}
	}
	m.Answers, m.Authority, m.Additional = sections[0], sections[1], sections[2]
	return m, nil
}

func readRecords(buf []byte, off int, n int) ([]Record, int, error) {
	var out []Record
	for i := 0; i < n; i++ {
		var rr Record
		var err error
		rr, off, err = readRecord(buf, off)
		if err != nil {
			return nil, off, err
		}
		out = append(out, rr)
	}
	return out, off, nil
}

func readRecord(buf []byte, off int) (Record, int, error) {
	var rr Record
	name, off, err := readName(buf, off)
	if err != nil {
		return rr, off, err
	}
	if off+10 > len(buf) {
		return rr, off, fmt.Errorf("dns: truncated resource record")
	}
	rr.Name = name
	rr.Type = RRType(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	off += 2 // class, always IN
	rr.TTL = binary.BigEndian.Uint32(buf[off : off+4])
	off += 4
	rdlength := int(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	if off+rdlength > len(buf) {
		return rr, off, fmt.Errorf("dns: truncated rdata for %s record", rr.Type)
	}
	rdata := buf[off : off+rdlength]
	rdEnd := off + rdlength

	switch rr.Type {
	case TypeA:
		if len(rdata) != 4 {
			return rr, off, fmt.Errorf("dns: invalid A rdata length %d", len(rdata))
		}
		rr.A = net.IP(rdata).String()
	case TypeAAAA:
		if len(rdata) != 16 {
			return rr, off, fmt.Errorf("dns: invalid AAAA rdata length %d", len(rdata))
		}
		rr.AAAA = net.IP(rdata).String()
	case TypeCNAME:
		rr.CNAME, _, err = readName(buf, off)
	case TypeNS:
		rr.NS, _, err = readName(buf, off)
	case TypePTR:
		rr.PTR, _, err = readName(buf, off)
	case TypeMX:
		if len(rdata) < 2 {
			return rr, off, fmt.Errorf("dns: invalid MX rdata")
		}
		mx := &MX{Preference: binary.BigEndian.Uint16(rdata[:2])}
		mx.Exchange, _, err = readName(buf, off+2)
		rr.MX = mx
	case TypeTXT:
		rr.TXT, err = decodeTXT(rdata)
	case TypeSRV:
		if len(rdata) < 6 {
			return rr, off, fmt.Errorf("dns: invalid SRV rdata")
		}
		srv := &SRV{
			Priority: binary.BigEndian.Uint16(rdata[0:2]),
			Weight:   binary.BigEndian.Uint16(rdata[2:4]),
			Port:     binary.BigEndian.Uint16(rdata[4:6]),
		}
		srv.Target, _, err = readName(buf, off+6)
		rr.SRV = srv
	case TypeCAA:
		if len(rdata) < 2 {
			return rr, off, fmt.Errorf("dns: invalid CAA rdata")
		}
		taglen := int(rdata[1])
		if 2+taglen > len(rdata) {
			return rr, off, fmt.Errorf("dns: invalid CAA tag length")
		}
		rr.CAA = &CAA{
			Flag:  rdata[0],
			Tag:   string(rdata[2 : 2+taglen]),
			Value: string(rdata[2+taglen:]),
		}
	case TypeSOA:
		soa := &SOA{}
		var next int
		soa.MName, next, err = readName(buf, off)
		if err == nil {
			soa.RName, next, err = readName(buf, next)
		}
		if err == nil {
			if next+20 > len(buf) {
				err = fmt.Errorf("dns: truncated SOA rdata")
			} else {
				soa.Serial = binary.BigEndian.Uint32(buf[next : next+4])
				soa.Refresh = binary.BigEndian.Uint32(buf[next+4 : next+8])
				soa.Retry = binary.BigEndian.Uint32(buf[next+8 : next+12])
				soa.Expire = binary.BigEndian.Uint32(buf[next+12 : next+16])
				soa.Minimum = binary.BigEndian.Uint32(buf[next+16 : next+20])
			}
		}
		rr.SOA = soa
	default:
		// Unknown/unsupported record type: keep the name/type/TTL, skip rdata.
	}
	if err != nil {
		return rr, off, err
	}
	return rr, rdEnd, nil
}

func decodeTXT(rdata []byte) ([]string, error) {
	var out []string
	for i := 0; i < len(rdata); {
		n := int(rdata[i])
		i++
		if i+n > len(rdata) {
			return nil, fmt.Errorf("dns: invalid TXT chunk length")
		}
		out = append(out, string(rdata[i:i+n]))
		i += n
	}
	return out, nil
}

// readName decodes a possibly-compressed domain name starting at off,
// returning the name and the offset immediately after it in the original
// message (i.e. after following any compression pointer, the returned offset
// is where the *next* field starts, not where the pointer chain ended).
func readName(buf []byte, off int) (string, int, error) {
	var labels []string
	origOff := -1
	cur := off
	jumps := 0
	for {
		if cur >= len(buf) {
			return "", 0, fmt.Errorf("dns: name extends past end of message")
		}
		length := int(buf[cur])
		switch {
		case length == 0:
			cur++
			if origOff == -1 {
				origOff = cur
			}
			if len(labels) == 0 {
				return "", origOff, nil
			}
			return strings.Join(labels, "."), origOff, nil
		case length&0xC0 == 0xC0:
			if cur+1 >= len(buf) {
				return "", 0, fmt.Errorf("dns: truncated compression pointer")
			}
			ptr := (int(length&0x3F) << 8) | int(buf[cur+1])
			if origOff == -1 {
				origOff = cur + 2
			}
			jumps++
			if jumps > 128 {
				return "", 0, fmt.Errorf("dns: too many compression pointer jumps")
			}
			cur = ptr
		default:
			start := cur + 1
			end := start + length
			if end > len(buf) {
				return "", 0, fmt.Errorf("dns: label extends past end of message")
			}
			labels = append(labels, string(buf[start:end]))
			cur = end
		}
	}
}
