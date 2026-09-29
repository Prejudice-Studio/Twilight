package api

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// 本文件用 TOML 解析器做结构化的密钥遮蔽与哨兵回填。
//
// 历史实现是逐行词法扫描：不认识多行字符串（"""…""" / '''…'''）、不认识
// section 头后面的注释（[Telegram] # bot），也不认识点号键和内联表。管理员可以
// 把 `emby_token = "<哨兵>"` 藏进某个普通字段的多行字符串里，PUT 时被逐行回填
// 成真值，GET 时又因为键名是普通字段而不遮蔽，从而读出任意密钥明文。
//
// 现在的做法：用 go-toml 的 unstable 解析器拿到每个字符串值在原文中的精确字节
// 区间和它的完整键路径（表头 + 点号键 + 内联表 + 数组下标），只在"键路径本身
// 是密钥"时才遮蔽或回填，替换时只改那一段字节，注释和排版原样保留。

// errTOMLSecretParse 表示内容不是合法 TOML，无法可靠遮蔽，调用方必须拒绝回传原文。
var errTOMLSecretParse = errors.New("config toml cannot be parsed for secret masking")

// tomlStringLeaf 是文档中的一个字符串值。
type tomlStringLeaf struct {
	path      []string // 小写键路径，数组元素以 "[i]" 表示
	raw       unstable.Range
	value     string
	sensitive bool
}

func (l tomlStringLeaf) pathKey() string { return strings.Join(l.path, "\x00") }

// secretConfigKeyNames 是 schema 中 Type=="secret" 字段的键名（小写）。
// "url" 过于通用，单独按父表判断，不放进"任意位置都算密钥"的集合。
func secretConfigKeyNames() map[string]bool {
	// Server-only keys are absent from the editable schema but still need masking.
	names := map[string]bool{"two_factor_key": true}
	for _, section := range configSectionDefs() {
		for _, field := range section.Fields {
			if field.Type == "secret" && field.Key != "url" {
				names[strings.ToLower(field.Key)] = true
			}
		}
	}
	return names
}

// tomlPathIsSecret 判断一个键路径是否属于密钥。config 读取器接受大量别名
// （根层裸键、[PostgreSQL].password、Global.database_url 等），因此按"最后一级
// 键名"判断，而不是只认 schema 的 section.field；宁可多遮一些——多遮的值在 PUT
// 时会按同一路径从磁盘原值回填，不会丢数据。
func tomlPathIsSecret(path []string, names map[string]bool) bool {
	if len(path) == 0 {
		return false
	}
	key := path[len(path)-1]
	// 数组元素继承父键的敏感性，由调用方传递，这里只看真正的键名。
	if strings.HasPrefix(key, "[") {
		return false
	}
	if names[key] {
		return true
	}
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "dsn", "private_key", "credential"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	switch key {
	case "database_url", "redis_url":
		return true
	case "url":
		if len(path) >= 2 {
			parent := path[len(path)-2]
			return parent == "database" || parent == "postgresql" || parent == "redis"
		}
	}
	return false
}

// scanTOMLStringLeaves 解析 content 并返回所有字符串值及其位置。
// 先用完整的 toml.Unmarshal 做语义校验（重复键、表重定义等），
// unstable 解析器只负责语法与位置。
func scanTOMLStringLeaves(content string) ([]tomlStringLeaf, error) {
	var tree map[string]any
	if err := toml.Unmarshal([]byte(content), &tree); err != nil {
		return nil, fmt.Errorf("%w: %v", errTOMLSecretParse, err)
	}
	names := secretConfigKeyNames()
	data := []byte(content)
	var parser unstable.Parser
	parser.Reset(data)
	var leaves []tomlStringLeaf
	var table []string
	arrayTableCount := map[string]int{}

	keyParts := func(it unstable.Iterator) []string {
		var parts []string
		for it.Next() {
			parts = append(parts, strings.ToLower(string(it.Node().Data)))
		}
		return parts
	}

	var walk func(node *unstable.Node, path []string, sensitive bool)
	walk = func(node *unstable.Node, path []string, sensitive bool) {
		if node == nil {
			return
		}
		sensitive = sensitive || tomlPathIsSecret(path, names)
		switch node.Kind {
		case unstable.String:
			leaves = append(leaves, tomlStringLeaf{
				path:      append([]string(nil), path...),
				raw:       node.Raw,
				value:     string(node.Data),
				sensitive: sensitive,
			})
		case unstable.Array:
			it := node.Children()
			i := 0
			for it.Next() {
				walk(it.Node(), append(append([]string(nil), path...), "["+strconv.Itoa(i)+"]"), sensitive)
				i++
			}
		case unstable.InlineTable:
			it := node.Children()
			for it.Next() {
				kv := it.Node()
				if kv.Kind != unstable.KeyValue {
					continue
				}
				child := append(append([]string(nil), path...), keyParts(kv.Key())...)
				walk(kv.Value(), child, sensitive)
			}
		}
	}

	for parser.NextExpression() {
		expr := parser.Expression()
		switch expr.Kind {
		case unstable.Table:
			table = keyParts(expr.Key())
		case unstable.ArrayTable:
			base := keyParts(expr.Key())
			joined := strings.Join(base, "\x00")
			idx := arrayTableCount[joined]
			arrayTableCount[joined] = idx + 1
			table = append(base, "["+strconv.Itoa(idx)+"]")
		case unstable.KeyValue:
			path := append(append([]string(nil), table...), keyParts(expr.Key())...)
			// 整张表（如 [Foo.secret]）位于密钥路径之下时，其所有值都视为密钥。
			walk(expr.Value(), path, tomlAnyAncestorSecret(table, names))
		}
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("%w: %v", errTOMLSecretParse, err)
	}
	return leaves, nil
}

func tomlAnyAncestorSecret(path []string, names map[string]bool) bool {
	for i := 1; i <= len(path); i++ {
		if tomlPathIsSecret(path[:i], names) {
			return true
		}
	}
	return false
}

type tomlByteEdit struct {
	raw         unstable.Range
	replacement string
}

func applyTOMLByteEdits(content string, edits []tomlByteEdit) string {
	if len(edits) == 0 {
		return content
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].raw.Offset > edits[j].raw.Offset })
	out := content
	for _, edit := range edits {
		start := int(edit.raw.Offset)
		end := start + int(edit.raw.Length)
		out = out[:start] + edit.replacement + out[end:]
	}
	return out
}

// tomlBasicString 把任意字符串编码成单行 TOML basic string。
// strconv.Quote 的 \x.. / \a 等转义不是合法 TOML，不能直接用于回填真值。
func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// maskTOMLSecrets 把 content 中所有位于密钥路径上的非空字符串替换为哨兵。
// 解析失败时返回错误：调用方不得把未遮蔽的原文回传给前端。
func maskTOMLSecrets(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}
	leaves, err := scanTOMLStringLeaves(content)
	if err != nil {
		return "", err
	}
	var edits []tomlByteEdit
	for _, leaf := range leaves {
		if !leaf.sensitive || leaf.value == "" {
			continue
		}
		edits = append(edits, tomlByteEdit{raw: leaf.raw, replacement: tomlBasicString(secretMaskValue)})
	}
	return applyTOMLByteEdits(content, edits), nil
}

// restoreTOMLSecrets 把提交内容里"位于密钥路径上、值恰为哨兵"的字符串回填为
// 磁盘上同一路径的真值。只按结构路径回填：哨兵出现在普通字段（包括多行字符串
// 内部）时保持原样，不会被换成任何真值。
//
// 回退：提交内容若是 GET 返回的规范化 content（密钥写在 [Section].field 规范
// 位置），而磁盘原文用的是别名位置（根层裸键等），同一路径查不到时退回
// fileValues[规范 section][field]——仍是同一个逻辑密钥，不会跨字段取值。
func restoreTOMLSecrets(submitted, disk string, fileValues map[string]map[string]any) (string, error) {
	if strings.TrimSpace(submitted) == "" {
		return submitted, nil
	}
	leaves, err := scanTOMLStringLeaves(submitted)
	if err != nil {
		return "", err
	}
	diskValues := map[string]string{}
	if strings.TrimSpace(disk) != "" {
		diskLeaves, err := scanTOMLStringLeaves(disk)
		if err == nil {
			for _, leaf := range diskLeaves {
				if leaf.sensitive {
					diskValues[leaf.pathKey()] = leaf.value
				}
			}
		}
	}
	var edits []tomlByteEdit
	for _, leaf := range leaves {
		if !leaf.sensitive || leaf.value != secretMaskValue {
			continue
		}
		real, found := diskValues[leaf.pathKey()]
		if !found {
			real = schemaSecretFallback(leaf.path, fileValues)
		}
		edits = append(edits, tomlByteEdit{raw: leaf.raw, replacement: tomlBasicString(real)})
	}
	return applyTOMLByteEdits(submitted, edits), nil
}

// schemaSecretFallback 只在路径正好对应 schema 的某个 secret 字段时取值：
// [Section].field（两级）或根层裸键 field（一级，对应唯一 secret 字段）。
func schemaSecretFallback(path []string, fileValues map[string]map[string]any) string {
	lookup := func(section, field string) string {
		if !isSecretField(section, field) {
			return ""
		}
		if text, ok := fileValues[section][field].(string); ok {
			return text
		}
		return ""
	}
	switch len(path) {
	case 2:
		return lookup(canonicalConfigSection(path[0]), path[1])
	case 1:
		match := ""
		count := 0
		for _, def := range configSectionDefs() {
			for _, field := range def.Fields {
				if field.Type == "secret" && field.Key == path[0] {
					match = lookup(def.Key, field.Key)
					count++
				}
			}
		}
		if count == 1 {
			return match
		}
	}
	return ""
}
