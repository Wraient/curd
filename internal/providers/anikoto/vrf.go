package anikoto

import (
	"encoding/base64"
	"net/url"
	"strings"
)

type vrfExchange struct {
	from string
	to   string
}

var vrfExchanges = []vrfExchange{
	{from: "AP6GeR8H0lwUz1", to: "UAz8Gwl10P6ReH"},
	{from: "1majSlPQd2M5", to: "da1l2jSmP5QM"},
	{from: "CPYvHj09Au3", to: "0jHA9CPYu3v"},
}

func encryptVRF(input string) string {
	return url.QueryEscape(encryptVRFRaw(input))
}

func encryptVRFRaw(input string) string {
	value := input
	value = exchangeVRF(value, vrfExchanges[0])
	value = rc4Encrypt("ItFKjuWokn4ZpB", value)
	value = rc4Encrypt("fOyt97QWFB3", value)
	value = exchangeVRF(value, vrfExchanges[1])
	value = exchangeVRF(value, vrfExchanges[2])
	value = reverseString(value)
	value = rc4Encrypt("736y1uTJpBLUX", value)
	return base64.URLEncoding.EncodeToString([]byte(value))
}

func exchangeVRF(input string, exchange vrfExchange) string {
	var result strings.Builder
	for _, char := range input {
		index := strings.IndexRune(exchange.from, char)
		if index >= 0 {
			result.WriteByte(exchange.to[index])
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func reverseString(input string) string {
	runes := []rune(input)
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}

func rc4Encrypt(key string, input string) string {
	var state [256]byte
	for index := range state {
		state[index] = byte(index)
	}
	keyBytes := []byte(key)
	j := 0
	for index := 0; index < len(state); index++ {
		j = (j + int(state[index]) + int(keyBytes[index%len(keyBytes)])) % 256
		state[index], state[j] = state[j], state[index]
	}
	result := make([]byte, len(input))
	i := 0
	j = 0
	for index := range input {
		i = (i + 1) % 256
		j = (j + int(state[i])) % 256
		state[i], state[j] = state[j], state[i]
		result[index] = input[index] ^ state[(int(state[i])+int(state[j]))%256]
	}
	return base64.URLEncoding.EncodeToString(result)
}
