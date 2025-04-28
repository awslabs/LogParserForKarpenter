// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nav-inc/datetime"
)

var header string = "nodeclaim,createdtime,nodepool,instancetypes,launchedtime,providerid,instancetype,zone,capacitytype,registeredtime,k8snodename,initializedtime,nodereadytime,nodereadytimesec,disruptiontime,disruptionreason,disruptiondecision,disruptednodecount,replacementnodecount,disruptedpodcount,annotationtime,annotation,tainttime,taint,interruptiontime,interruptionkind,deletedtime,nodeterminationtime,nodeterminationtimesec,nodelifecycletime,nodelifecycletimesec,initialized,deleted"

// keep disruptednodecount, replacementnodecount, disruptedpodcount as strings because then we can have empty string ("") to differ from real values
type Nodeclaimstruct struct {
	/*
		createdtime, nodepool, instancetypes, launchedtime, providerid, instancetype, zone, capacitytype, registeredtime, k8snodename, initializedtime string
		disruptiontime, disruptionreason, disruptiondecision, disruptednodecount, replacementnodecount, disruptedpodcount                              string
		annotationtime, annotation, tainttime, taint, interruptiontime, interruptionkind, deletedtime                                                  string
		nodereadytime, nodeterminationtime, nodelifecycletime                                                                                          time.Duration
		nodereadytimesec, nodeterminationtimesec, nodelifecycletimesec                                                                                 float64
		initialized, deleted
	*/
	Createdtime            string
	Nodepool               string
	Instancetypes          string
	Launchedtime           string
	Providerid             string
	Instancetype           string
	Zone                   string
	Capacitytype           string
	Registeredtime         string
	K8snodename            string
	Initializedtime        string
	Nodereadytime          time.Duration
	Nodereadytimesec       float64
	Disruptiontime         string
	Disruptionreason       string
	Disruptiondecision     string
	Disruptednodecount     string
	Replacementnodecount   string
	Disruptedpodcount      string
	Annotationtime         string
	Annotation             string
	Tainttime              string
	Taint                  string
	Interruptiontime       string
	Interruptionkind       string
	Deletedtime            string
	Nodeterminationtime    time.Duration
	Nodeterminationtimesec float64
	Nodelifecycletime      time.Duration
	Nodelifecycletimesec   float64
	Initialized            bool
	Deleted                bool
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
		headerSlice[index] = fmt.Sprintf("%s[%d]", element, index+1)
	}
	return strings.Join(headerSlice, ",")
}

// internal helper function for scanner error handling
func scannerErr(scanner *bufio.Scanner, stdin string) {
	// Ctrl-C will always lead to "http2: response body closed", so suppress this error
	if err := scanner.Err(); err != nil {
		if err.Error() != "http2: response body closed" {
			fmt.Fprintf(os.Stderr, "Error \"%s\" parsing %s\n", err, stdin)
		}
	}
}

// internal helper function to populate nodeclaimmap from K8s ConfigMap data i.e. map[string]string
// reflect package requires exported fields as well
func Populatenodeclaimmap(nodeclaimmap *map[string]Nodeclaimstruct, cmdata map[string]string) {
	var nodeclaimstruct Nodeclaimstruct

	v := reflect.ValueOf(nodeclaimstruct)
	t := reflect.TypeOf(nodeclaimstruct)

	for i := range t.NumField() {
		field := t.Field(i)
		value := v.Field(i)
		fmt.Printf("Field Name: %s, Field Type: %s, Field Value: %v\n",
			field.Name, field.Type, value)
	}

	for key, val := range cmdata {
		for ind, attr := range strings.SplitN(val, ",", -1) {
			field := t.Field(ind)
			//reflect.ValueOf(&nodeclaimstruct).Elem().FieldByName(field.Name).Set(reflect.ValueOf(field.Type))
			fmt.Printf("nodeclaim (key): %s, ind: %d field name: %s field type: %s attr: %s\n", key, ind, field.Name, field.Type, attr)
		}
		fmt.Printf("nodeclaim (key): %s, value: %v\n", key, val)
		//(*nodeclaimmap)[key] = nodeclaimstruct
	}
}

// wrapper around main parsing logic with blocking
func BlockingParser(ch chan os.Signal, scanner *bufio.Scanner, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, stdin string, inputline int) {
	// main parsing logic
	for scanner.Scan() {
		//logline := scanner.Text()
		ParseKarpenterLogs(scanner.Text(), nodeclaimmap, k8snodenamemap, stdin, inputline)
		// we wait until Ctrl-C because we have an input from something like "kubectl logs -n karpenter -l=app.kubernetes.io/name=karpenter -f"
		go func() {
			<-ch
		}()
	}
	scannerErr(scanner, stdin)
}

// wrapper around main parsing logic without blocking
func NonBlockingParser(scanner *bufio.Scanner, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, stdin string, inputline int) {
	// main parsing logic
	for scanner.Scan() {
		//logline := scanner.Text()
		ParseKarpenterLogs(scanner.Text(), nodeclaimmap, k8snodenamemap, stdin, inputline)
	}
	scannerErr(scanner, stdin)
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
					Createdtime:            createdtime,
					Nodepool:               nodepool,
					Instancetypes:          instancetypes,
					Launchedtime:           "",
					Providerid:             "",
					Instancetype:           "",
					Zone:                   "",
					Capacitytype:           "",
					Registeredtime:         "",
					K8snodename:            "",
					Initializedtime:        "",
					Nodereadytime:          0,
					Nodereadytimesec:       0.0,
					Disruptiontime:         "",
					Disruptionreason:       "",
					Disruptiondecision:     "",
					Disruptednodecount:     "",
					Replacementnodecount:   "",
					Disruptedpodcount:      "",
					Annotationtime:         "",
					Annotation:             "",
					Tainttime:              "",
					Taint:                  "",
					Interruptionkind:       "",
					Deletedtime:            "",
					Nodeterminationtime:    0,
					Nodeterminationtimesec: 0.0,
					Nodelifecycletime:      0,
					Nodelifecycletimesec:   0.0,
					Initialized:            false,
					Deleted:                false,
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
								entry.Launchedtime = val
							case 2:
								awsproviderID := strings.Split(val, "/")
								entry.Providerid = awsproviderID[len(awsproviderID)-1]
							case 3:
								entry.Instancetype = val
							case 4:
								entry.Zone = val
							case 5:
								entry.Capacitytype = val
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
								entry.Registeredtime = val
							case 2:
								entry.K8snodename = val
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
						if entry.Initializedtime = matchslicesub[1]; entry.Initializedtime != "" {
							// calculate node startup time
							if entry.Createdtime != "" {
								t1, _ := datetime.Parse(entry.Createdtime, time.UTC)
								t2, _ := datetime.Parse(entry.Initializedtime, time.UTC)
								entry.Nodereadytime = t2.Sub(t1)
								entry.Nodereadytimesec = entry.Nodereadytime.Seconds()
							}
						} else {
							fmt.Fprintf(os.Stderr, "Parsing error empty \"initialized time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
						}
						// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
						entry.Initialized = true
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
								entry.Disruptiontime = val
							case 1:
								entry.Disruptionreason = val
							case 2:
								entry.Disruptiondecision = val
							case 3:
								entry.Disruptednodecount = val
							case 4:
								entry.Replacementnodecount = val
							case 5:
								entry.Disruptedpodcount = val
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
								entry.Interruptiontime = val
							case 1:
								entry.Interruptionkind = val
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
								entry.Annotationtime = val
							case 2:
								// annotation key
								entry.Annotation = val
							case 3:
								// add taint value to already existing taint key
								entry.Annotation = fmt.Sprintf("%s:%s", entry.Annotation, val)
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
								entry.Tainttime = val
							case 2:
								// taint key
								entry.Taint = val
							case 3:
								// add taint value to already existing taint key
								// note: taint value might be an empty string, so we reverse our check logic here !!!
								entry.Taint = fmt.Sprintf("%s:%s", entry.Taint, val)
							case 4:
								// add taint effect to already existing taint key:value
								entry.Taint = fmt.Sprintf("%s:%s", entry.Taint, val)
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
										entry.Tainttime = val
									case 2:
										// taint key
										entry.Taint = val
									case 3:
										// add taint value to already existing taint key
										// note: taint value might be an empty string
										entry.Taint = fmt.Sprintf("%s:%s", entry.Taint, val)
									case 4:
										// add taint effect to already existing taint key:value
										entry.Taint = fmt.Sprintf("%s:%s", entry.Taint, val)
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
											entry.Tainttime = val
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
						if entry.Deletedtime = matchslicesub[1]; entry.Deletedtime != "" {
							// calculate node lifecycle time
							if entry.Createdtime != "" {
								t1, _ := datetime.Parse(entry.Createdtime, time.UTC)
								t2, _ := datetime.Parse(entry.Deletedtime, time.UTC)
								entry.Nodelifecycletime = t2.Sub(t1)
								entry.Nodelifecycletimesec = entry.Nodelifecycletime.Seconds()
							}
							// calculate node termination time (time it takes from lifecycle annotation to actual deletion)
							// if this takes really long you might have some blocking PDB or taints
							if entry.Annotationtime != "" {
								t1, _ := datetime.Parse(entry.Annotationtime, time.UTC)
								t2, _ := datetime.Parse(entry.Deletedtime, time.UTC)
								entry.Nodeterminationtime = t2.Sub(t1)
								entry.Nodeterminationtimesec = entry.Nodeterminationtime.Seconds()
							}
						} else {
							fmt.Fprintf(os.Stderr, "Parsing error empty \"deleted time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
						}
						// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
						entry.Deleted = true
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
		return s[i].value.Createdtime < s[j].value.Createdtime
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
			// probably use "reflection" here as well later !
			fmt.Printf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%.1f,%t,%t\n", v.key, v.value.Createdtime, v.value.Nodepool, v.value.Instancetypes, v.value.Launchedtime, v.value.Providerid, v.value.Instancetype, v.value.Zone, v.value.Capacitytype, v.value.Registeredtime, v.value.K8snodename, v.value.Initializedtime, v.value.Nodereadytime, v.value.Nodereadytimesec, v.value.Disruptiontime, v.value.Disruptionreason, v.value.Disruptiondecision, v.value.Disruptednodecount, v.value.Replacementnodecount, v.value.Disruptedpodcount, v.value.Annotationtime, v.value.Annotation, v.value.Tainttime, v.value.Taint, v.value.Interruptiontime, v.value.Interruptionkind, v.value.Deletedtime, v.value.Nodeterminationtime, v.value.Nodeterminationtimesec, v.value.Nodelifecycletime, v.value.Nodelifecycletimesec, v.value.Initialized, v.value.Deleted)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}
}

func ConvertResult(nodeclaimmap *map[string]Nodeclaimstruct) map[string]string {
	keyvalueMap := make(map[string]string)

	if len((*nodeclaimmap)) != 0 {
		s := sortResult(nodeclaimmap)

		// Each key must consist of alphanumeric characters, '-', '_' or '.' so nodeclaim names must comply (add a check later
		// add all information as key-value
		for _, v := range s {
			keyvalueMap[v.key] = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f,%s,%.1f,%t,%t\n", v.value.Createdtime, v.value.Nodepool, v.value.Instancetypes, v.value.Launchedtime, v.value.Providerid, v.value.Instancetype, v.value.Zone, v.value.Capacitytype, v.value.Registeredtime, v.value.K8snodename, v.value.Initializedtime, v.value.Nodereadytime, v.value.Nodereadytimesec, v.value.Disruptiontime, v.value.Disruptionreason, v.value.Disruptiondecision, v.value.Disruptednodecount, v.value.Replacementnodecount, v.value.Disruptedpodcount, v.value.Annotationtime, v.value.Annotation, v.value.Tainttime, v.value.Taint, v.value.Interruptiontime, v.value.Interruptionkind, v.value.Deletedtime, v.value.Nodeterminationtime, v.value.Nodeterminationtimesec, v.value.Nodelifecycletime, v.value.Nodelifecycletimesec, v.value.Initialized, v.value.Deleted)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nNo results - empty \"nodeclaim\" map\n")
	}

	return keyvalueMap
}
