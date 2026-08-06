package interner

type StringInterner map[string]string

func (si StringInterner) Intern(s string) string {
	if s == "" {
		return ""
	}
	if cached, ok := si[s]; ok {
		return cached
	}
	si[s] = s
	return s
}
