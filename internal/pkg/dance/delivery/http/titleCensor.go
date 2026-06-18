package http

import (
	"strings"
	"unicode/utf8"
)

const maxDanceTitleLen = 60

var bannedWords = []string{
	"хуй", "хуя", "хуйн", "пизд", "ёбан", "ебан", "ебат", "ебал",
	"нахуй", "блядь", "блять", "сука", "мудак", "мудил", "мразь",
	"шлюх", "залуп", "манда", "уёбок", "уебок", "долбоёб", "долбоеб",
	"пиздабол", "гандон", "пиздюк", "ёблан", "хуила", "заёбан",
	"ублюдок", "ёбнут", "пиздёж",
	"fuck", "shit", "bitch", "asshole", "cunt", "nigger", "nigga",
	"faggot", "motherfuck", "cocksucker", "whore", "slut",
}

func sanitizeDanceTitle(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxDanceTitleLen {
		runes := []rune(s)
		s = string(runes[:maxDanceTitleLen])
	}
	return s
}

func containsBannedWords(s string) bool {
	lower := strings.ToLower(s)
	for _, w := range bannedWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
