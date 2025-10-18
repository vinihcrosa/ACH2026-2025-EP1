package utils

import "encoding/json"

func ParseData(input interface{}, out interface{}) {
	bytes, _ := json.Marshal(input)
	json.Unmarshal(bytes, out)
}
