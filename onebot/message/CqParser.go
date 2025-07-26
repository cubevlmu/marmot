package message

import (
	"github.com/tidwall/gjson"
	"marmot/utils"
	"strings"
)

// ParseMessageFromString parses msg as type string to a sort of MessageSegment.
// msg is the value of key "message" of the data unmarshalled from the
// API response JSON.
//
// Parse CQ-String to message
func ParseMessageFromString(raw string) (m Message) {
	var seg Segment
	var k string
	m = Message{}
	for raw != "" {
		i := 0
		for i < len(raw) && !(raw[i] == '[' && i+4 < len(raw) && raw[i:i+4] == "[CQ:") {
			i++
		}

		if i > 0 {
			m = append(m, Text(UnescapeCQText(raw[:i])))
		}

		if i+4 > len(raw) {
			return
		}

		raw = raw[i+4:] // skip "[CQ:"
		i = 0
		for i < len(raw) && raw[i] != ',' && raw[i] != ']' {
			i++
		}

		if i+1 > len(raw) {
			return
		}
		seg.Type = raw[:i]
		seg.Data = make(map[string]string)
		raw = raw[i:]
		i = 0

		for {
			if raw[0] == ']' {
				m = append(m, seg)
				raw = raw[1:]
				break
			}
			raw = raw[1:]

			for i < len(raw) && raw[i] != '=' {
				i++
			}
			if i+1 > len(raw) {
				return
			}
			k = raw[:i]
			raw = raw[i+1:] // skip "="
			i = 0
			for i < len(raw) && raw[i] != ',' && raw[i] != ']' {
				i++
			}

			if i+1 > len(raw) {
				return
			}
			seg.Data[k] = UnescapeCQCodeText(raw[:i])
			raw = raw[i:]
			i = 0
		}
	}
	return m
}

func ParseMessage(msg []byte) Message {
	x := gjson.Parse(utils.BytesToString(msg))
	if x.IsArray() {
		return ParseMessageFromArray(x)
	}
	return ParseMessageFromString(x.String())
}

func parse2map(val gjson.Result) map[string]string {
	m := map[string]string{}
	val.ForEach(func(key, value gjson.Result) bool {
		m[key.String()] = value.String()
		return true
	})
	return m
}

func ParseMessageFromArray(msgs gjson.Result) Message {
	message := Message{}
	msgs.ForEach(func(_, item gjson.Result) bool {
		message = append(message, Segment{
			Type: item.Get("type").String(),
			Data: parse2map(item.Get("data")),
		})
		return true
	})
	return message
}

// ExtractPlainText Extract plain text from string
func (m Message) ExtractPlainText() string {
	sb := strings.Builder{}
	for _, val := range m {
		if val.Type == "text" {
			sb.WriteString(val.Data["text"])
		}
	}
	return sb.String()
}
