// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nav-inc/datetime"
)

var header string = "nodeclaim,createdtime,nodepool,instancetypes,launchedtime,providerid,instancetype,zone,capacitytype,registeredtime,k8snodename,initializedtime,nodereadytime,nodereadytimesec,disruptiontime,disruptionreason,disruptiondecision,disruptednodecount,replacementnodecount,disruptedpodcount,annotationtime,annotation,tainttime,taint,interruptiontime,interruptionkind,deletedtime,nodeterminationtime,nodeterminationtimesec,nodelifecycletime,nodelifecycletimesec,initialized,deleted"

// keep disruptednodecount, replacementnodecount, disruptedpodcount as strings because then we can have empty string ("") to differ from real values
type Nodeclaimstruct struct {
	createdtime, nodepool, instancetypes, launchedtime, providerid, instancetype, zone, capacitytype, registeredtime, k8snodename, initializedtime string
	disruptiontime, disruptionreason, disruptiondecision, disruptednodecount, replacementnodecount, disruptedpodcount                              string
	annotationtime, annotation, tainttime, taint, interruptiontime, interruptionkind, deletedtime                                                  string
	nodereadytime, nodeterminationtime, nodelifecycletime                                                                                          time.Duration
	nodereadytimesec, nodeterminationtimesec, nodelifecycletimesec                                                                                 float64
	initialized, deleted                                                                                                                           bool
}

// struct for further sorting of map
type keyvalue struct {
	key   string
	value Nodeclaimstruct
}

// internal helper function for pattern matching
func matchPattern(pattern, logline string) []string {
	re := regexp.MustCompile(pattern)
	return re.FindStringSubmatch(logline)
}

// internal helper function for header indexing
func headerIndex() string {
	headerSlice := strings.Split(header, ",")
	for index, element := range headerSlice {
		// Go slices start with index 0, but Linux utils like awk count from 1
		headerSlice[index] = fmt.Sprintf("%s(%d)", element, index+1)
	}
	return strings.Join(headerSlice, ",")
}

// helper function index header without nodeclaim
func headerRemain() string {
	headerSlice := strings.Split(headerIndex(), ",")
	return strings.Join(headerSlice[1:], ",")
}

// main parsing logic
func ParseKarpenterLogs(logline string, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, filename string, inputline int) {
	var createdtime, nodepool, instancetypes, nodeclaim string
	var matchslice []string
	var re *regexp.Regexp
	// replacer replaces commas which are followed by a space with a dash and removes spaces.
	// It's a package-level variable so we can easily reuse it, but
	// this program doesn't take advantage of that fact.
	var replacer = strings.NewReplacer(", ", "|", " ", "", "(s)", "s")

	inputline++
	re = regexp.MustCompile(`"message":"(.*)","commit"`)
	matchslice = re.FindStringSubmatch(logline)
	// process matchslice if we found a match
	if matchslice != nil {
		//fmt.Println("message: ", matchslice[1])
		switch matchslice[1] {
		case "created nodeclaim":
			// extract time and nodeclaim (new one)
			pattern := `"time":"(.*)","logger".*"NodePool":{"name":"(.*)"},"NodeClaim":{"name":"(.*)"},"requests".*"instance-types":"(.*)"}`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				for i, val := range matchslicesub[1:] {
					switch i {
					case 0:
						createdtime = val
					case 1:
						nodepool = val
					case 2:
						nodeclaim = val
					case 3:
						// substitute "," because we output CSV finally
						// Karpenter provisioner.go prints the first 5 instance types only and remaining number
						if strings.Contains(val, " and ") {
							position := strings.LastIndex(val, " and ")
							instancetypes = fmt.Sprintf("%s|%s", replacer.Replace(val[:position]), replacer.Replace(val[position:]))
						} else {
							instancetypes = replacer.Replace(val)
						}
					}
				}
				// we only create a new nodeclaimmap map entry when we capture a "created nodeclaim" log line
				// add entry to hash map
				(*nodeclaimmap)[nodeclaim] = Nodeclaimstruct{
					createdtime:            createdtime,
					nodepool:               nodepool,
					instancetypes:          instancetypes,
					launchedtime:           "",
					providerid:             "",
					instancetype:           "",
					zone:                   "",
					capacitytype:           "",
					registeredtime:         "",
					k8snodename:            "",
					initializedtime:        "",
					nodereadytime:          0,
					nodereadytimesec:       0.0,
					disruptiontime:         "",
					disruptionreason:       "",
					disruptiondecision:     "",
					disruptednodecount:     "",
					replacementnodecount:   "",
					disruptedpodcount:      "",
					annotationtime:         "",
					annotation:             "",
					tainttime:              "",
					taint:                  "",
					interruptionkind:       "",
					deletedtime:            "",
					nodeterminationtime:    0,
					nodeterminationtimesec: 0.0,
					nodelifecycletime:      0,
					nodelifecycletimesec:   0.0,
					initialized:            false,
					deleted:                false,
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "launched nodeclaim":
			// extract all nodeclaim details here
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},.*"provider-id":"(.*)","instance-type":"(.*)","zone":"(.*)","capacity-type":"(.*)","allocatable"`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						//matchslice[0] always contains whole logline
						for i, val := range matchslicesub[1:] {
							switch i {
							case 0:
								entry.launchedtime = val
							case 2:
								awsproviderID := strings.Split(val, "/")
								entry.providerid = awsproviderID[len(awsproviderID)-1]
							case 3:
								entry.instancetype = val
							case 4:
								entry.zone = val
							case 5:
								entry.capacitytype = val
							}
						}
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "registered nodeclaim":
			// extract time, nodeclaim and K8s node name
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},.*,"Node":{"name":"(.*)"}`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						//matchslicesub[0] always contains whole logline
						for i, val := range matchslicesub[1:] {
							switch i {
							case 0:
								entry.registeredtime = val
							case 2:
								entry.k8snodename = val
								(*k8snodenamemap)[val] = nodeclaim
							}
						}
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "initialized nodeclaim":
			// extract time and nodeclaim
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace"`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						//matchslicesub[0] always contains whole logline
						if entry.initializedtime = matchslicesub[1]; entry.initializedtime != "" {
							// calculate node startup time
							if entry.createdtime != "" {
								t1, _ := datetime.Parse(entry.createdtime, time.UTC)
								t2, _ := datetime.Parse(entry.initializedtime, time.UTC)
								entry.nodereadytime = t2.Sub(t1)
								entry.nodereadytimesec = entry.nodereadytime.Seconds()
							}
						} else {
							fmt.Fprintf(os.Stderr, "Parsing error empty \"initialized time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
						}
						// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
						entry.initialized = true
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "disrupting node(s)":
			// extract time, message reason, decision, disrupted-node-count, replacment-node-count, podcount and nodeclaim (this message kind has NodeClaim in a different position!)d nodeclaim (this message kind has NodeClaim in a different position!)
			pattern := `"time":"(.*)","logger".*"reason":"(.*)","decision":"(.*)","disrupted-node-count":(.*),"replacement-node-count":(.*),"pod-count":(.*),"disrupted-nodes":.*,"NodeClaim":{"name":"(.*)"},"capacity-type"`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[3] will contain NodeClaim
				if nodeclaim = matchslicesub[7]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						for i, val := range matchslicesub[1:] {
							//fmt.Println("ind: ", i, "val: ", val)
							switch i {
							case 0:
								entry.disruptiontime = val
							case 1:
								entry.disruptionreason = val
							case 2:
								entry.disruptiondecision = val
							case 3:
								entry.disruptednodecount = val
							case 4:
								entry.replacementnodecount = val
							case 5:
								entry.disruptedpodcount = val
								/*
									if entry.disruptedpodcount, err = strconv.Atoi(val); err != nil {
										entry.replacementnodecount = 0
								*/
							}
						}
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "initiating delete from interruption message":
			// extract time, message kind (interruption kind/reason) and nodeclaim (this message kind has NodeClaim in a different position!)
			pattern := `"time":"(.*)","logger".*"messageKind":"(.*)","NodeClaim":{"name":"(.*)"},"action"`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[3] will contain NodeClaim
				if nodeclaim = matchslicesub[3]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						for i, val := range matchslicesub[1:] {
							switch i {
							case 0:
								entry.interruptiontime = val
							case 1:
								entry.interruptionkind = val
							}
							//fmt.Println("ind: ", i, "val: ", val)
						}
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "annotated nodeclaim":
			// extract time, nodeclaim and annotation key/value
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace".*,"(.*)":"(.*)"}`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[3] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						for i, val := range matchslicesub[1:] {
							switch i {
							case 0:
								entry.annotationtime = val
							case 2:
								// annotation key
								entry.annotation = val
							case 3:
								// add taint value to already existing taint key
								entry.annotation = fmt.Sprintf("%s:%s", entry.annotation, val)
							}
							(*nodeclaimmap)[nodeclaim] = entry
						}
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "tainted node":
			// extract time, nodeclaim and taint key/value/effect for Karpenter version 1.1.x+
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"taint.Key":"(.*)","taint.Value":"(.*)","taint.Effect":"(.*)"}`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[3] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					//log.Fatal("Parsing error while extracting NodeClaim, probably Karpenter log syntax has changed!")
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						for i, val := range matchslicesub[1:] {
							switch i {
							case 0:
								entry.tainttime = val
							case 2:
								// taint key
								entry.taint = val
							case 3:
								// add taint value to already existing taint key
								// note: taint value might be an empty string, so we reverse our check logic here !!!
								entry.taint = fmt.Sprintf("%s:%s", entry.taint, val)
							case 4:
								// add taint effect to already existing taint key:value
								entry.taint = fmt.Sprintf("%s:%s", entry.taint, val)
							}
							if val == "" && i != 3 {
								fmt.Fprintf(os.Stderr, "Parsing error empty \"K8s node name\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
							}
							(*nodeclaimmap)[nodeclaim] = entry
						}
					}
				}
			} else {
				// Karpenter version 0.37.x and 1.0.x don't put nodeclaim into "tainted node" message !
				// extract time, k8snodename taint key/value/effect for Karpenter version 1.0.x
				pattern := `"time":"(.*)","logger".*"Node":{"name":"(.*)"},"namespace".*,"taint.Key":"(.*)","taint.Value":"(.*)","taint.Effect":"(.*)"}`
				if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
					// if logline parsing went well, matchslicesub[2] will contain K8s node name
					if k8snodename := matchslicesub[2]; k8snodename == "" {
						fmt.Fprintf(os.Stderr, "Parsing error empty \"K8s node name\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
					} else {
						if nodeclaim, ok := (*k8snodenamemap)[k8snodename]; ok {
							if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
								for i, val := range matchslicesub[1:] {
									switch i {
									case 0:
										entry.tainttime = val
									case 2:
										// taint key
										entry.taint = val
									case 3:
										// add taint value to already existing taint key
										// note: taint value might be an empty string
										entry.taint = fmt.Sprintf("%s:%s", entry.taint, val)
									case 4:
										// add taint effect to already existing taint key:value
										entry.taint = fmt.Sprintf("%s:%s", entry.taint, val)
									}
									(*nodeclaimmap)[nodeclaim] = entry
								}
							}
						} else {
							fmt.Fprintf(os.Stderr, "No corresponding \"NodeClaim\" for K8s node \"%s\" for message \"tainted node\" in line %d in %s\n", k8snodename, inputline, filename)
							fmt.Fprintf(os.Stderr, "Most probably %s does not contain a corresponding \"created nodeclaim\" log entry\n", filename)
						}
					}
				} else {
					// Karpenter version 0.37.x don't put taint key/value/effect into "tainted node" message !
					//
					// extract time and k8snodename for Karpenter version 0.37
					pattern := `"time":"(.*)","logger".*"Node":{"name":"(.*)"},"namespace"`
					if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
						// if logline parsing went well, matchslicesub[2] will contain K8s node name
						if k8snodename := matchslicesub[2]; k8snodename == "" {
							fmt.Fprintf(os.Stderr, "Parsing error empty \"K8s node name\" for message \"%s\" in line %d in %s\n", matchslice[1], inputline, filename)
						} else {
							if nodeclaim, ok := (*k8snodenamemap)[k8snodename]; ok {
								if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
									for i, val := range matchslicesub[1:] {
										switch i {
										case 0:
											entry.tainttime = val
										}
										(*nodeclaimmap)[nodeclaim] = entry
									}
								} else {
									fmt.Fprintf(os.Stderr, "No corresponding \"NodeClaim\" for K8s node \"%s\" for message \"tainted node\" in line %d in %s\n", k8snodename, inputline, filename)
									fmt.Fprintf(os.Stderr, "Most probably %s does not contain a corresponding \"created nodeclaim\" log entry\n", filename)
								}
							}
						}
					} else {
						fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
					}
				}
			}
		case "deleted nodeclaim":
			// extract time and nodeclaim
			pattern := `"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace"`
			if matchslicesub := matchPattern(pattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else {
					if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
						//matchslicesub[0] always contains whole logline
						if entry.deletedtime = matchslicesub[1]; entry.deletedtime != "" {
							// calculate node lifecycle time
							if entry.createdtime != "" {
								t1, _ := datetime.Parse(entry.createdtime, time.UTC)
								t2, _ := datetime.Parse(entry.deletedtime, time.UTC)
								entry.nodelifecycletime = t2.Sub(t1)
								entry.nodelifecycletimesec = entry.nodelifecycletime.Seconds()
							}
							// calculate node termination time (time it takes from lifecycle annotation to actual deletion)
							// if this takes really long you might have some blocking PDB or taints
							if entry.annotationtime != "" {
								t1, _ := datetime.Parse(entry.annotationtime, time.UTC)
								t2, _ := datetime.Parse(entry.deletedtime, time.UTC)
								entry.nodeterminationtime = t2.Sub(t1)
								entry.nodeterminationtimesec = entry.nodeterminationtime.Seconds()
							}
						} else {
							fmt.Fprintf(os.Stderr, "Parsing error empty \"deleted time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
						}
						// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
						entry.deleted = true
						(*nodeclaimmap)[nodeclaim] = entry
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		}
	}
	//}
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
		return s[i].value.createdtime < s[j].value.createdtime
	})

	return s
}

func PrintSortedResult(nodeclaimmap *map[string]Nodeclaimstruct) {
	if len((*nodeclaimmap)) != 0 {
		s := sortResult(nodeclaimmap)

		// iterate over the slice to get the desired order of nodeclaim by createdtime
		// offset 1 to make post-processing with tools like aws easier
		fmt.Println(headerIndex())

		for _, v := range s {
			//fmt.Println(v.key, "->", v.value)
			fmt.Printf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%.1f,%t,%t\n", v.key, v.value.createdtime, v.value.nodepool, v.value.instancetypes, v.value.launchedtime, v.value.providerid, v.value.instancetype, v.value.zone, v.value.capacitytype, v.value.registeredtime, v.value.k8snodename, v.value.initializedtime, v.value.nodereadytime, v.value.nodereadytimesec, v.value.disruptiontime, v.value.disruptionreason, v.value.disruptiondecision, v.value.disruptednodecount, v.value.replacementnodecount, v.value.disruptedpodcount, v.value.annotationtime, v.value.annotation, v.value.tainttime, v.value.taint, v.value.interruptiontime, v.value.interruptionkind, v.value.deletedtime, v.value.nodeterminationtime, v.value.nodeterminationtimesec, v.value.nodelifecycletime, v.value.nodelifecycletimesec, v.value.initialized, v.value.deleted)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}
}

func ConvertResult(nodeclaimmap *map[string]Nodeclaimstruct) map[string]string {
	keyvalueMap := make(map[string]string)

	if len((*nodeclaimmap)) != 0 {
		s := sortResult(nodeclaimmap)

		// add header
		// Each key must consist of alphanumeric characters, '-', '_' or '.' so nodeclaim names must comply (add a check later)
		keyvalueMap["nodeclaim"] = headerRemain()
		// add all information as key-value
		for _, v := range s {
			keyvalueMap[v.key] = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%.1f,%t,%t\n", v.value.createdtime, v.value.nodepool, v.value.instancetypes, v.value.launchedtime, v.value.providerid, v.value.instancetype, v.value.zone, v.value.capacitytype, v.value.registeredtime, v.value.k8snodename, v.value.initializedtime, v.value.nodereadytime, v.value.nodereadytimesec, v.value.disruptiontime, v.value.disruptionreason, v.value.disruptiondecision, v.value.disruptednodecount, v.value.replacementnodecount, v.value.disruptedpodcount, v.value.annotationtime, v.value.annotation, v.value.tainttime, v.value.taint, v.value.interruptiontime, v.value.interruptionkind, v.value.deletedtime, v.value.nodeterminationtime, v.value.nodeterminationtimesec, v.value.nodelifecycletime, v.value.nodelifecycletimesec, v.value.initialized, v.value.deleted)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}

	return keyvalueMap
}
