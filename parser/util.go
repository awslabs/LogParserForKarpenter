// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
)

// struct for further sorting of map
type keyvalue struct {
	key   string
	value Nodeclaimstruct
}

// internal helper function to set header based on Nodeclaimstruct
func init() {
	var nodeclaimstruct Nodeclaimstruct
	reflecttype := reflect.TypeOf(nodeclaimstruct)
	header = "Nodeclaim[1]"
	for i := range reflecttype.NumField() {
		header = fmt.Sprintf("%s,%s[%d]", header, reflecttype.Field(i).Name, i+2)
	}
}

// internal helper function to populate nodeclaimmap from K8s ConfigMap data i.e. map[string]string
func Populatenodeclaimmap(nodeclaimmap *map[string]Nodeclaimstruct, cmdata map[string]string) {
	for key, val := range cmdata {
		var nodeclaimstruct Nodeclaimstruct
		if err := json.Unmarshal([]byte(val), &nodeclaimstruct); err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error while encoding Nodeclaimstruct of nodeclaim \"%s\\n", key)
		}
		(*nodeclaimmap)[key] = nodeclaimstruct
	}
}

// helper function sorted slice - sort the nodeclaimmap map by createdtime if not empty
func sortResult(nodeclaimmap *map[string]Nodeclaimstruct) []keyvalue {
	s := make([]keyvalue, 0, len((*nodeclaimmap)))
	for k, v := range *nodeclaimmap {
		s = append(s, keyvalue{k, v})
	}
	sort.SliceStable(s, func(i, j int) bool {
		return s[i].value.Createdtime < s[j].value.Createdtime
	})
	return s
}

func PrintSortedResult(nodeclaimmap *map[string]Nodeclaimstruct) {
	if len((*nodeclaimmap)) == 0 {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
		return
	}
	s := sortResult(nodeclaimmap)
	fmt.Println(header)
	reflectval := reflect.ValueOf(Nodeclaimstruct{})
	for _, v := range s {
		fmt.Print(v.key)
		for i := range reflectval.NumField() {
			fmt.Print(",", reflect.ValueOf(v.value).Field(i).Interface())
		}
		fmt.Println()
	}
}

// ConvertToCSV converts nodeclaimmap to a CSV string with header
func ConvertToCSV(nodeclaimmap *map[string]Nodeclaimstruct) string {
	var csvBuffer bytes.Buffer

	// Write header
	csvBuffer.WriteString(header)
	csvBuffer.WriteString("\n")

	if len(*nodeclaimmap) == 0 {
		return csvBuffer.String()
	}

	// Sort and write data
	s := sortResult(nodeclaimmap)

	for _, v := range s {
		csvBuffer.WriteString(v.key)

		reflectval := reflect.ValueOf(v.value)
		for i := range reflectval.NumField() {
			csvBuffer.WriteString(fmt.Sprintf(",%v", reflectval.Field(i).Interface()))
		}
		csvBuffer.WriteString("\n")
	}

	return csvBuffer.String()
}

// ConvertResult is used by k8s package to create ConfigMap data
func ConvertResult(nodeclaimmap *map[string]Nodeclaimstruct) map[string]string {
	keyvalueMap := make(map[string]string)
	if len((*nodeclaimmap)) == 0 {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
		return keyvalueMap
	}
	s := sortResult(nodeclaimmap)
	for _, v := range s {
		if jsondata, err := json.Marshal(v.value); err == nil {
			keyvalueMap[v.key] = string(jsondata)
		} else {
			fmt.Fprintf(os.Stderr, "JSON encoding error while encoding Nodeclaimstruct of nodeclaim \"%s\\n", v.key)
		}
	}
	return keyvalueMap
}
