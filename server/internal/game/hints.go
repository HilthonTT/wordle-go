package game

import (
	"slices"
	"sort"
	"strings"
)

const (
	englishAlphabets = "abcdefghijklmnopqrstuvwxyz"
	maxHints         = 5
)

func generateHints(dictionary []string, attempts []Attempt, alphasStatusMap map[string]CharacterStatus) []string {
	alphasInCorrectLocation := make(map[string]bool)
	alphasInWrongLocation := make(map[string]bool)
	alphasNotPresent := make(map[string]bool)
	alphasPresent := make(map[string]bool)
	alphasUnknown := make(map[string]bool)
	for _, char := range englishAlphabets {
		charStr := string(char)
		switch alphasStatusMap[charStr] {
		case Unknown:
			alphasUnknown[charStr] = true
		case NotPresent:
			alphasNotPresent[charStr] = true
		case WrongLocation:
			alphasInWrongLocation[charStr] = true
			alphasPresent[charStr] = true
		case CorrectLocation:
			alphasInCorrectLocation[charStr] = true
			alphasPresent[charStr] = true
		}
	}

	var words []string
	maxWordLength := calculateMaximumWordLength(dictionary)
	maxWordLength75Percent := maxWordLength * 75 / 100
	if len(alphasUnknown) > 20 && len(alphasInCorrectLocation) >= maxWordLength75Percent {
		words = findWordsWithMostUnknownLetters(dictionary, alphasUnknown)
	}

	if len(words) == 0 {
		words = filterWordsWithLettersNotPresent(dictionary, alphasNotPresent)
		words = filterWordsWithLettersInWrongLocations(words, attempts)
		words = filterWordsWithoutLetters(words, alphasPresent)
	}

	if len(words) > 1 {
		missingLetters := findMissingLetters(words, alphasInCorrectLocation)
		differingLetters := findDifferingLetters(words)
		if len(alphasInCorrectLocation) >= maxWordLength75Percent && len(words) >= maxHints-1 {
			words = findWordsWithMostMissingLetters(dictionary, missingLetters)
		} else if len(alphasInCorrectLocation) < maxWordLength75Percent && len(differingLetters) >= maxHints-1 {
			words = findWordsWithMostMissingLetters(dictionary, differingLetters)
		} else {
			freqMap := buildCharacterFrequencyMap(words)
			sort.SliceStable(words, func(i, j int) bool {
				iFreq := calculateFrequencyValue(words[i], freqMap)
				jFreq := calculateFrequencyValue(words[j], freqMap)
				if iFreq == jFreq {
					return words[i] < words[j]
				}
				return iFreq > jFreq
			})
		}
	}

	words = filterWordsAlreadyAttempted(words, attempts)
	if len(words) > maxHints {
		return words[:maxHints]
	}
	return words
}

// buildCharacterFrequencyMap maps byte→count instead of string→count, avoiding
// a string allocation per character in the hot path.
func buildCharacterFrequencyMap(words []string) map[byte]int {
	freq := make(map[byte]int)
	for _, word := range words {
		for i := 0; i < len(word); i++ {
			freq[word[i]]++
		}
	}
	return freq
}

func buildKnownCharacterLocationMap(attempts []Attempt) (map[string][]int, map[string][]int) {
	correctLocationMap, incorrectLocationMap := make(map[string][]int), make(map[string][]int)

	for _, attempt := range attempts {
		for idx := 0; idx < len(attempt.Answer); idx++ {
			charStr := string(attempt.Answer[idx])
			switch attempt.Result[idx] {
			case CorrectLocation:
				if !slices.Contains(correctLocationMap[charStr], idx) { // replaces the hasValue closure
					correctLocationMap[charStr] = append(correctLocationMap[charStr], idx)
				}
			case WrongLocation:
				if !slices.Contains(incorrectLocationMap[charStr], idx) {
					incorrectLocationMap[charStr] = append(incorrectLocationMap[charStr], idx)
				}
			}
		}
	}
	return correctLocationMap, incorrectLocationMap
}

// calculateFrequencyValue uses a [26]bool instead of map[string]bool for the
// "seen" set — zero allocation, O(1) lookup, no string conversions.
func calculateFrequencyValue(word string, freqMap map[byte]int) int {
	var seen [26]bool
	val := 0
	for i := 0; i < len(word); i++ {
		b := word[i]
		idx := b - 'a'
		if !seen[idx] {
			val += freqMap[b]
			seen[idx] = true
		}
	}
	return val
}

func calculateMaximumWordLength(words []string) int {
	max := 0
	for _, word := range words {
		if len(word) > max {
			max = len(word)
		}
	}
	return max
}

// countUniqueLetters replaces the map[string]bool with a [26]bool array.
func countUniqueLetters(word string) int {
	var seen [26]bool
	count := 0
	for i := 0; i < len(word); i++ {
		idx := word[i] - 'a'
		if !seen[idx] {
			seen[idx] = true
			count++
		}
	}
	return count
}

// filterWordsAlreadyAttempted builds a set from attempts once for O(1) lookup
// instead of the previous O(n*m) nested loop.
func filterWordsAlreadyAttempted(words []string, attempts []Attempt) []string {
	attempted := make(map[string]struct{}, len(attempts))
	for _, a := range attempts {
		attempted[a.Answer] = struct{}{}
	}

	result := make([]string, 0, len(words))
	for _, word := range words {
		if _, found := attempted[word]; !found {
			result = append(result, word)
		}
	}
	return result
}

func filterWordsWithLettersInWrongLocations(words []string, attempts []Attempt) []string {
	correctLocationMap, incorrectLocationMap := buildKnownCharacterLocationMap(attempts)

	hasCharacterInWrongLocation := func(word string) bool {
		for char, indices := range correctLocationMap {
			for _, idx := range indices {
				if string(word[idx]) != char {
					return true
				}
			}
		}
		for char, indices := range incorrectLocationMap {
			for _, idx := range indices {
				if string(word[idx]) == char {
					return true
				}
			}
		}
		return false
	}

	result := make([]string, 0, len(words))
	for _, word := range words {
		if !hasCharacterInWrongLocation(word) {
			result = append(result, word)
		}
	}
	return result
}

func filterWordsWithLettersNotPresent(words []string, lettersMap map[string]bool) []string {
	result := make([]string, 0, len(words))
	for _, word := range words {
		if !hasLetterFrom(word, lettersMap) {
			result = append(result, word)
		}
	}
	return result
}

// hasLetterFrom reports whether word contains any letter in the map.
func hasLetterFrom(word string, lettersMap map[string]bool) bool {
	for i := 0; i < len(word); i++ {
		if lettersMap[string(word[i])] {
			return true
		}
	}
	return false
}

func filterWordsWithoutLetters(words []string, lettersMap map[string]bool) []string {
	result := make([]string, 0, len(words))
	for _, word := range words {
		if hasAllLetters(word, lettersMap) {
			result = append(result, word)
		}
	}
	return result
}

// hasAllLetters reports whether word contains every letter in the map.
func hasAllLetters(word string, lettersMap map[string]bool) bool {
	for char := range lettersMap {
		if !strings.Contains(word, char) {
			return false
		}
	}
	return true
}

func findDifferingLetters(words []string) map[string]bool {
	letterCountMap := make(map[string]int)
	for _, word := range words {
		var seen [26]bool // was map[string]bool allocated per word
		for i := 0; i < len(word); i++ {
			b := word[i]
			idx := b - 'a'
			if !seen[idx] {
				letterCountMap[string(b)]++
				seen[idx] = true
			}
		}
	}

	result := make(map[string]bool)
	for char, count := range letterCountMap {
		if count < 2 {
			result[char] = true
		}
	}
	return result
}

func findMissingLetters(words []string, letterMap map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for _, word := range words {
		for i := 0; i < len(word); i++ {
			charStr := string(word[i])
			if !letterMap[charStr] {
				result[charStr] = true
			}
		}
	}
	return result
}

func findWordsWithMostMissingLetters(words []string, lettersMap map[string]bool) []string {
	missingLettersScore := func(word string) int {
		var seen [26]bool // was map[string]bool allocated per scoring call
		score := 0
		for i := 0; i < len(word); i++ {
			b := word[i]
			idx := b - 'a'
			if !seen[idx] {
				if lettersMap[string(b)] {
					score++
				}
				seen[idx] = true
			}
		}
		return score
	}

	sort.SliceStable(words, func(i, j int) bool {
		iScore := missingLettersScore(words[i])
		jScore := missingLettersScore(words[j])
		if iScore == jScore {
			return words[i] < words[j]
		}
		return iScore > jScore
	})
	return words
}

func findWordsWithMostUnknownLetters(words []string, lettersMap map[string]bool) []string {
	hasAllUnknownLetters := func(word string) bool {
		for i := 0; i < len(word); i++ {
			if !lettersMap[string(word[i])] {
				return false
			}
		}
		return true
	}

	result := make([]string, 0, len(words))
	for _, word := range words {
		if hasAllUnknownLetters(word) {
			result = append(result, word)
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		iCount := countUniqueLetters(result[i])
		jCount := countUniqueLetters(result[j])
		if iCount == jCount {
			return result[i] < result[j]
		}
		return iCount > jCount
	})
	return result
}
