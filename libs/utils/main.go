package utils

import "encoding/json"

func ParseData(input interface{}, out interface{}) error {
	bytes, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, out)
}
