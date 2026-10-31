// 最小 telnet 协议 (RFC 854) IAC 处理: 剥离服务端协商序列并生成拒绝响应.
package telnet

// telnet 协议字节常量
const (
	iacSE   byte = 240 // 子协商结束
	iacNOP  byte = 241
	iacSB   byte = 250 // 子协商开始
	iacWILL byte = 251
	iacWONT byte = 252
	iacDO   byte = 253
	iacDONT byte = 254
	iacIAC  byte = 255 // 转义前缀
)

// stripIAC 从 telnet 数据流中剥离协商序列, 返回净荷与待回写的协商响应.
//
// 处理规则 (最小集):
//   - IAC IAC → 字面量 0xFF
//   - IAC DO + option → 消费, 回 IAC WONT option (拒绝)
//   - IAC WILL + option → 消费, 回 IAC DONT option (拒绝)
//   - IAC DONT / WONT + option → 消费 (无需响应)
//   - IAC SB ... IAC SE → 消费整个子协商 (含 IAC IAC 转义)
//
// 简化: 协商序列跨读包截断时丢弃不完整尾部 (下次重新解析), 不做跨包续接.
func stripIAC(data []byte) (payload []byte, reply []byte) {
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b != iacIAC {
			payload = append(payload, b)
			continue
		}
		if i+1 >= len(data) {
			break
		}
		cmd := data[i+1]
		switch cmd {
		case iacIAC:
			payload = append(payload, iacIAC)
			i++
		case iacDO:
			if i+2 < len(data) {
				reply = append(reply, iacIAC, iacWONT, data[i+2])
				i += 2
			} else {
				i++
			}
		case iacWILL:
			if i+2 < len(data) {
				reply = append(reply, iacIAC, iacDONT, data[i+2])
				i += 2
			} else {
				i++
			}
		case iacDONT, iacWONT:
			if i+2 < len(data) {
				i += 2
			} else {
				i++
			}
		case iacSB:
			j := i + 2
			for j < len(data) {
				if data[j] == iacIAC {
					// IAC IAC 转义跳过; IAC SE 结束子协商
					if j+1 < len(data) && data[j+1] == iacIAC {
						j += 2
						continue
					}
					if j+1 < len(data) && data[j+1] == iacSE {
						break
					}
				}
				j++
			}
			i = j + 1 // 指向 SE 之后 (未找到 SE 则越过整包)
		default:
			i++ // NOP/AYT 等两字节命令直接消费
		}
	}
	return payload, reply
}
