package sessions

import (
	"encoding/binary"
	"errors"
	"slices"
)

type protoField struct {
	Number int
	Wire   int
	Value  []byte
	Raw    []byte
}

func protoFields(data []byte) ([]protoField, error) {
	var out []protoField
	for offset := 0; offset < len(data); {
		start := offset
		tag, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return nil, errors.New("Antigravity 摘要缓存字段无效")
		}
		offset += n
		field := protoField{Number: int(tag >> 3), Wire: int(tag & 7)}
		if field.Number <= 0 || field.Number >= 1<<29 {
			return nil, errors.New("Antigravity 摘要缓存编号无效")
		}
		switch field.Wire {
		case 0:
			_, n = binary.Uvarint(data[offset:])
			if n <= 0 {
				return nil, errors.New("Antigravity 摘要缓存数值无效")
			}
			offset += n
		case 1:
			offset += 8
		case 5:
			offset += 4
		case 2:
			size, n := binary.Uvarint(data[offset:])
			if n <= 0 || size > uint64(len(data)) {
				return nil, errors.New("Antigravity 摘要缓存长度无效")
			}
			offset += n
			end := offset + int(size)
			if end > len(data) {
				return nil, errors.New("Antigravity 摘要缓存不完整")
			}
			field.Value = data[offset:end]
			offset = end
		default:
			return nil, errors.New("Antigravity 摘要缓存结构需要适配")
		}
		if offset > len(data) {
			return nil, errors.New("Antigravity 摘要缓存不完整")
		}
		field.Raw = slices.Clone(data[start:offset])
		out = append(out, field)
	}
	return out, nil
}

func protoEntryID(field protoField) (string, error) {
	if field.Number != 1 {
		return "", nil
	}
	if field.Wire != 2 {
		return "", errors.New("Antigravity 摘要缓存结构需要适配")
	}
	fields, err := protoFields(field.Value)
	if err != nil {
		return "", err
	}
	id := ""
	for _, item := range fields {
		if item.Number == 1 && item.Wire == 2 {
			if id != "" {
				return "", errors.New("Antigravity 摘要缓存键重复")
			}
			id = string(item.Value)
		}
	}
	if !sessionIDPattern.MatchString(id) {
		return "", errors.New("Antigravity 摘要缓存会话 ID 无效")
	}
	return id, nil
}

func cutAgyCache(data []byte, ids map[string]bool) ([]byte, []byte, error) {
	fields, err := protoFields(data)
	if err != nil {
		return nil, nil, err
	}
	var remaining, removed []byte
	for _, field := range fields {
		id, err := protoEntryID(field)
		if err != nil {
			return nil, nil, err
		}
		if ids[id] {
			removed = append(removed, field.Raw...)
		} else {
			remaining = append(remaining, field.Raw...)
		}
	}
	return remaining, removed, nil
}

func restoreAgyCache(current, removed []byte) ([]byte, error) {
	fields, err := protoFields(current)
	if err != nil {
		return nil, err
	}
	existing := map[string][]byte{}
	for _, field := range fields {
		id, err := protoEntryID(field)
		if err != nil {
			return nil, err
		}
		if id != "" {
			existing[id] = field.Raw
		}
	}
	restore, err := protoFields(removed)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(current)
	for _, field := range restore {
		id, err := protoEntryID(field)
		if err != nil {
			return nil, err
		}
		if old, ok := existing[id]; ok {
			if !slices.Equal(old, field.Raw) {
				return nil, errors.New("Antigravity 已存在同 ID 的摘要，请先处理冲突")
			}
			continue
		}
		out = append(out, field.Raw...)
	}
	return out, nil
}
