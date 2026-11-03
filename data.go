package operations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

func dataOps() []*Operation {
	return []*Operation{jsonFormat, jsonToYAML, yamlToJSON, hexToRGB}
}

var jsonIndentArg = selectArg("indent", "Indentation", "2",
	Option{"2", "2 spaces"},
	Option{"4", "4 spaces"},
	Option{"tab", "Tab"},
	Option{"minify", "None, on a single line"},
)

var jsonIndents = map[string]string{"2": "  ", "4": "    ", "tab": "\t", "minify": ""}

func checkJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

var jsonToken = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|[{}\[\],:]|[^\s{}\[\],:"]+`)

// Values are not decoded, so large numbers and duplicate keys survive.
func formatJSON(text []byte, indent string) ([]byte, error) {
	if err := checkJSON(text); err != nil {
		return nil, err
	}
	tokens := jsonToken.FindAll(text, -1)
	var out []byte
	depth := 0
	newline := func() {
		if indent != "" {
			out = append(out, '\n')
			out = append(out, strings.Repeat(indent, depth)...)
		}
	}
	at := func(i int) byte {
		if i < 0 || i >= len(tokens) {
			return 0
		}
		return tokens[i][0]
	}
	for i, t := range tokens {
		switch string(t) {
		case "{", "[":
			out = append(out, t...)
			if next := at(i + 1); next != '}' && next != ']' {
				depth++
				newline()
			}
		case "}", "]":
			if prev := at(i - 1); prev != '{' && prev != '[' {
				depth--
				newline()
			}
			out = append(out, t...)
		case ",":
			out = append(out, ',')
			newline()
		case ":":
			out = append(out, ':')
			if indent != "" {
				out = append(out, ' ')
			}
		default:
			out = append(out, t...)
		}
	}
	return out, nil
}

func jsonString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

var jsonFormat = &Operation{
	ID:          "json-format",
	Name:        "JSON format",
	Description: "Pretty-prints or minifies JSON. Numbers are kept digit for digit, and keys in their original order.",
	Category:    DataFormat,
	Output:      BinaryOutput,
	Aliases:     []string{"json beautify", "json prettify", "json minify", "jq"},
	InfoURL:     "https://www.json.org",
	Args:        []ArgDef{jsonIndentArg},
	Examples: []Example{
		{Input: `{"a":1,"b":[true,null],"c":{}}`, Output: "{\n  \"a\": 1,\n  \"b\": [\n    true,\n    null\n  ],\n  \"c\": {}\n}"},
		{Input: "{ \"id\": 12345678901234567890,\n  \"tags\": [ \"x\" ] }", Args: Args{"indent": "minify"}, Output: `{"id":12345678901234567890,"tags":["x"]}`},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return formatJSON(in, jsonIndents[a.String("indent")])
	},
}

var jsonToYAML = &Operation{
	ID:          "json-to-yaml",
	Name:        "JSON to YAML",
	Description: `Converts JSON to YAML. Strings that some YAML parsers would read as another type, such as "yes", are quoted.`,
	Category:    DataFormat,
	Output:      BinaryOutput,
	Aliases:     []string{"yaml"},
	Inverse:     "yaml-to-json",
	InfoURL:     "https://yaml.org",
	Examples: []Example{
		{Input: `{"name":"ops","tags":["a","b"],"count":1}`, Output: "name: ops\ntags:\n  - a\n  - b\ncount: 1\n"},
		{Input: `{"enabled":"yes"}`, Output: "enabled: \"yes\"\n"},
	},
	Run: func(in []byte, _ Args) ([]byte, error) {
		if err := checkJSON(in); err != nil {
			return nil, err
		}
		doc, err := jsonNode(in)
		if err != nil {
			return nil, err
		}
		quoteYAML11(doc)
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(doc); err != nil {
			return nil, err
		}
		enc.Close()
		return buf.Bytes(), nil
	},
}

// Read as booleans, numbers or merge keys by YAML 1.1 parsers.
var yaml11Special = regexp.MustCompile(`^(?:<<|y|Y|yes|Yes|YES|n|N|no|No|NO|on|On|ON|off|Off|OFF|[-+]?\d[\d_]*(?::[0-5]?\d)+(?:\.[\d_]*)?)$`)

// The encoder only quotes strings that YAML 1.2 would read as another type.
// It also writes tabs in block scalars, which no parser reads back.
func quoteYAML11(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!str" && (yaml11Special.MatchString(n.Value) || strings.Contains(n.Value, "\t")) {
		n.Style = yaml.DoubleQuotedStyle
	}
	for _, c := range n.Content {
		quoteYAML11(c)
	}
}

// jsonNode reads valid JSON into YAML nodes, keeping key order and numbers
// as written.
func jsonNode(in []byte) (*yaml.Node, error) {
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.UseNumber()
	var read func() (*yaml.Node, error)
	read = func() (*yaml.Node, error) {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case json.Delim:
			n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			if t == '{' {
				n.Kind, n.Tag = yaml.MappingNode, "!!map"
			}
			for dec.More() {
				if n.Kind == yaml.MappingNode {
					k, _ := dec.Token()
					n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k.(string)})
				}
				c, err := read()
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, c)
			}
			dec.Token()
			if len(n.Content) == 0 {
				n.Style = yaml.FlowStyle
			}
			return n, nil
		case string:
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}, nil
		case json.Number:
			tag := "!!int"
			if strings.ContainsAny(string(t), ".eE") {
				tag = "!!float"
			}
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: string(t)}, nil
		case bool:
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(t)}, nil
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	n, err := read()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{n}}, nil
}

var yamlToJSON = &Operation{
	ID:          "yaml-to-json",
	Name:        "YAML to JSON",
	Description: "Converts a YAML document to JSON. Numbers are kept digit for digit, and keys in their original order.",
	Category:    DataFormat,
	Output:      BinaryOutput,
	Aliases:     []string{"yml", "yaml"},
	Inverse:     "json-to-yaml",
	InfoURL:     "https://yaml.org",
	Args:        []ArgDef{jsonIndentArg},
	Examples: []Example{
		{Input: "name: ops\ntags: [a, b]\nn: 1\n", Args: Args{"indent": "minify"}, Output: `{"name":"ops","tags":["a","b"],"n":1}`},
		{Input: "a:\n  - true\n  - ~\n", Output: "{\n  \"a\": [\n    true,\n    null\n  ]\n}"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		var doc yaml.Node
		if err := yaml.Unmarshal(in, &doc); err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
		var out []byte
		if err := appendNodeJSON(&out, &doc); err != nil {
			return nil, err
		}
		return formatJSON(out, jsonIndents[a.String("indent")])
	},
}

func appendNodeJSON(out *[]byte, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			*out = append(*out, "null"...)
			return nil
		}
		return appendNodeJSON(out, n.Content[0])
	case yaml.AliasNode:
		return appendNodeJSON(out, n.Alias)
	case yaml.SequenceNode:
		*out = append(*out, '[')
		for i, c := range n.Content {
			if i > 0 {
				*out = append(*out, ',')
			}
			if err := appendNodeJSON(out, c); err != nil {
				return err
			}
		}
		*out = append(*out, ']')
	case yaml.MappingNode:
		*out = append(*out, '{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind == yaml.AliasNode {
				key = key.Alias
			}
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: only scalar keys can be converted to JSON", key.Line)
			}
			if i > 0 {
				*out = append(*out, ',')
			}
			*out = append(*out, jsonString(key.Value)...)
			*out = append(*out, ':')
			if err := appendNodeJSON(out, n.Content[i+1]); err != nil {
				return err
			}
		}
		*out = append(*out, '}')
	case yaml.ScalarNode:
		return appendScalarJSON(out, n)
	default:
		*out = append(*out, "null"...)
	}
	return nil
}

var jsonNumber = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$`)

func appendScalarJSON(out *[]byte, n *yaml.Node) error {
	switch n.ShortTag() {
	case "!!null":
		*out = append(*out, "null"...)
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return err
		}
		*out = strconv.AppendBool(*out, b)
	case "!!int", "!!float":
		if jsonNumber.MatchString(n.Value) {
			*out = append(*out, n.Value...)
			return nil
		}
		if i, ok := new(big.Int).SetString(strings.ReplaceAll(strings.TrimPrefix(n.Value, "+"), "_", ""), 0); ok {
			*out = i.Append(*out, 10)
			return nil
		}
		var f float64
		if err := n.Decode(&f); err != nil {
			return err
		}
		// JSON has no NaN or Infinity.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			*out = append(*out, "null"...)
			return nil
		}
		*out = strconv.AppendFloat(*out, f, 'g', -1, 64)
	default:
		*out = append(*out, jsonString(n.Value)...)
	}
	return nil
}

var hexColor = regexp.MustCompile(`(?i)^(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$`)

func toRGB(color, format string) (string, error) {
	h := strings.TrimPrefix(strings.TrimSpace(color), "#")
	if !hexColor.MatchString(h) {
		return "", fmt.Errorf("invalid hex color %q: expected #rgb, #rgba, #rrggbb or #rrggbbaa", strings.TrimSpace(color))
	}
	if len(h) <= 4 {
		var full []byte
		for i := range len(h) {
			full = append(full, h[i], h[i])
		}
		h = string(full)
	}
	var values []string
	for i := 0; i < 6; i += 2 {
		v, _ := strconv.ParseUint(h[i:i+2], 16, 8)
		values = append(values, strconv.FormatUint(v, 10))
	}
	if len(h) == 8 {
		v, _ := strconv.ParseUint(h[6:8], 16, 8)
		values = append(values, strconv.FormatFloat(math.Round(float64(v)/255*1000)/1000, 'f', -1, 64))
	}
	joined := strings.Join(values, ", ")
	switch {
	case format == "values":
		return joined, nil
	case len(values) == 4:
		return "rgba(" + joined + ")", nil
	default:
		return "rgb(" + joined + ")", nil
	}
}

var hexToRGB = &Operation{
	ID:          "hex-to-rgb",
	Name:        "Hex to RGB",
	Description: `Converts hex colors (one per line) to RGB. e.g. "#ff8800" becomes "rgb(255, 136, 0)".`,
	Category:    DataFormat,
	Output:      BinaryOutput,
	Aliases:     []string{"color", "colour", "css color"},
	InfoURL:     "https://developer.mozilla.org/docs/Web/CSS/hex-color",
	Args: []ArgDef{
		selectArg("format", "Format", "css",
			Option{"css", "CSS: rgb(255, 136, 0) or rgba(255, 136, 0, 0.5)"},
			Option{"values", "Values: 255, 136, 0"},
		),
	},
	Examples: []Example{
		{Input: "#ff8800", Output: "rgb(255, 136, 0)"},
		{Input: "#f80\n#FF880080", Output: "rgb(255, 136, 0)\nrgba(255, 136, 0, 0.502)"},
		{Input: "ff8800", Args: Args{"format": "values"}, Output: "255, 136, 0"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		var err error
		out := mapLines(string(in), func(lines []string) []string {
			for i, line := range lines {
				if strings.TrimSpace(line) != "" && err == nil {
					lines[i], err = toRGB(line, a.String("format"))
				}
			}
			return lines
		})
		if err != nil {
			return nil, err
		}
		return []byte(out), nil
	},
}
