package sessions

import "context"

func scanParts(ctx context.Context, s Session, visit func(Part) bool) (int64, error) {
	if s.Provider == "claude" || s.Provider == "codex" {
		next, _, err := readRecords(ctx, s.Path, 0, func(r record, _ int64) bool {
			for _, part := range recordParts(r, s) {
				if !visit(part) {
					return false
				}
			}
			return true
		})
		return next, err
	}
	parts, err := storedParts(ctx, s)
	if err != nil {
		return 0, err
	}
	for i, part := range parts {
		if !visit(part) {
			return int64(i + 1), nil
		}
	}
	return int64(len(parts)), nil
}

func storedParts(ctx context.Context, s Session) ([]Part, error) {
	if s.Provider == "opencode" {
		return openCodeTranscript(ctx, s)
	}
	steps, err := readAgySteps(ctx, s.Paths)
	if err != nil {
		return nil, err
	}
	return agyParts(steps), nil
}

func providerTranscript(ctx context.Context, s Session, cursor int64) (Transcript, error) {
	if s.Provider == "claude" || s.Provider == "codex" {
		return readTranscript(ctx, s, cursor)
	}
	parts, err := storedParts(ctx, s)
	if err != nil {
		return Transcript{}, err
	}
	if cursor < 0 || cursor > int64(len(parts)) {
		return Transcript{}, unsupportedStore(s.Provider)
	}
	end := min(len(parts), int(cursor)+100)
	page := append([]Part{}, parts[int(cursor):end]...)
	return Transcript{Parts: page, NextCursor: int64(end), HasMore: end < len(parts)}, nil
}
