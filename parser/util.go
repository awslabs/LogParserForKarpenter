// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
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

	// loop over v.value, which is a Nodeclaimstruct, using reflect
	reflecttype := reflect.TypeOf(nodeclaimstruct)

	// Go slices start with index 0, but Linux utils like awk count from 1
	header = "Nodeclaim[1]"

	for i := range reflecttype.NumField() {
		//header = header + "," + reflecttype.Field(i).Name + "[" + strconv.Itoa(i+2) + "]"
		header = fmt.Sprintf("%s,%s[%d]", header, reflecttype.Field(i).Name, i+2)
	}
}

// internal helper function to populate nodeclaimmap from K8s ConfigMap data i.e. map[string]string
func Populatenodeclaimmap(nodeclaimmap *map[string]Nodeclaimstruct, cmdata map[string]string) {
	var nodeclaimstruct Nodeclaimstruct

	for key, val := range cmdata {
		err := json.Unmarshal([]byte(val), &nodeclaimstruct)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error while encoding Nodeclaimstruct of nodeclaim \"%s\\n", key)
		}
		(*nodeclaimmap)[key] = nodeclaimstruct
	}
}

// helper function sorted slice
func sortResult(nodeclaimmap *map[string]Nodeclaimstruct) []keyvalue {
	// sort the nodeclaimmap map by createdtime if not empty

	// create an empty helper slice s of key-value pairs
	s := make([]keyvalue, 0, len((*nodeclaimmap)))

	// append all map key-value pairs to the slice
	for k, v := range *nodeclaimmap {
		s = append(s, keyvalue{k, v})
	}

	sort.SliceStable(s, func(i, j int) bool {
		return s[i].value.Createdtime < s[j].value.Createdtime
	})

	return s
}

func PrintSortedResult(nodeclaimmap *map[string]Nodeclaimstruct) {
	if len((*nodeclaimmap)) != 0 {
		s := sortResult(nodeclaimmap)

		// print header
		fmt.Println(header)

		// print nodeclaim with attributes
		for _, v := range s {
			fmt.Print(v.key)

			// loop over v.value, which is a Nodeclaimstruct, using reflect
			reflectval := reflect.ValueOf(v.value)

			for i := range reflectval.NumField() {
				fmt.Print(",", reflectval.Field(i).Interface())
			}
			fmt.Println()
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}
}

// used by k8s package to create ConfigMap data
func ConvertResult(nodeclaimmap *map[string]Nodeclaimstruct) map[string]string {
	keyvalueMap := make(map[string]string)

	if len((*nodeclaimmap)) != 0 {
		s := sortResult(nodeclaimmap)

		// Each key must consist of alphanumeric characters, '-', '_' or '.' so nodeclaim names must comply (add a check later
		// add all information as key-value
		for _, v := range s {
			jsondata, err := json.Marshal(v.value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "JSON encoding error while encoding Nodeclaimstruct of nodeclaim \"%s\\n", v.key)
			}
			keyvalueMap[v.key] = string(jsondata)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}

	return keyvalueMap
}
