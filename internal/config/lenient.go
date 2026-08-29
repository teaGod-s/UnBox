package config

import "bytes"

// Lenient 将 FongMi多线路 生态中常见的非标准 JSON 转换为标准 JSON。
//
// 处理四类问题：
//  1. // 行注释与 /* */ 块注释
//  2. 数组/对象的尾随逗号
//  3. 字符串字面量内的裸控制字符（制表符、换行）
//  4. UTF-8 BOM
//
// 实现为单遍字符扫描器，全程跟踪是否处于字符串字面量内部。
// 这一点不可用正则替代：真实样本中 441 个 "//" 里有 433 个是 URL 协议分隔符。
func Lenient(raw []byte) []byte {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	var out bytes.Buffer
	out.Grow(len(raw))

	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if inString {
			switch {
			case escaped:
				// Previous char was \; decide whether to write it based on current char
				if c == '\t' {
					out.WriteByte('\\')
					out.WriteByte('t')
				} else if c == '\n' {
					out.WriteByte('\\')
					out.WriteByte('n')
				} else if c == '\r' {
					out.WriteByte('\\')
					out.WriteByte('r')
				} else if c < 0x20 {
					// Other control chars after backslash - discard both backslash and control char
					// (don't write anything)
				} else {
					// Normal character - write backslash and character
					out.WriteByte('\\')
					out.WriteByte(c)
				}
				escaped = false
			case c == '\\':
				// Defer writing the backslash until we see what follows
				escaped = true
			case c == '"':
				out.WriteByte(c)
				inString = false
			case c == '\t':
				out.WriteString(`\t`)
			case c == '\n':
				out.WriteString(`\n`)
			case c == '\r':
				out.WriteString(`\r`)
			case c < 0x20:
				// 其余控制字符直接丢弃
			default:
				out.WriteByte(c)
			}
			continue
		}

		// 以下为字符串外部
		if c == '"' {
			out.WriteByte(c)
			inString = true
			continue
		}

		// 行注释
		if c == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			continue
		}

		// 块注释
		if c == '/' && i+1 < len(raw) && raw[i+1] == '*' {
			i += 2
			for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
				i++
			}
			i++ // 循环的 i++ 会跳过 '/'
			continue
		}

		// 尾随逗号：向前看，若下一个非空白字符是 } 或 ]，则丢弃该逗号
		if c == ',' {
			j := skipWhitespaceAndComments(raw, i+1)
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}

		out.WriteByte(c)
	}

	return out.Bytes()
}

func skipWhitespaceAndComments(raw []byte, i int) int {
	for i < len(raw) {
		if isSpace(raw[i]) {
			i++
		} else if i+1 < len(raw) && raw[i] == '/' && raw[i+1] == '/' {
			// Skip line comment until newline
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			// Skip the newline if present
			if i < len(raw) && raw[i] == '\n' {
				i++
			}
		} else if i+1 < len(raw) && raw[i] == '/' && raw[i+1] == '*' {
			// Skip block comment
			i += 2
			for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
				i++
			}
			if i+1 < len(raw) {
				i += 2 // Skip the */
			}
		} else {
			break
		}
	}
	return i
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
