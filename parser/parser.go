// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nav-inc/datetime"
)

// var header string = "nodeclaim,createdtime,nodepool,instancetypes,launchedtime,providerid,instancetype,zone,capacitytype,registeredtime,k8snodename,initializedtime,nodereadytime,nodereadytimesec,disruptiontime,disruptionreason,disruptiondecision,disruptednodecount,replacementnodecount,disruptedpodcount,annotationtime,annotation,tainttime,taint,interruptiontime,interruptionkind,deletedtime,nodeterminationtime,nodeterminationtimesec,nodelifecycletime,nodelifecycletimesec,initialized,deleted"
var header string

// export all struct values because this is required for usage with packages like JSON encoding/decoding or reflect
// keep disruptednodecount, replacementnodecount, disruptedpodcount as strings because then we can have empty string ("") to differ from real values
type Nodeclaimstruct struct {
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

// internal helper function for pattern matching
func matchPattern(pattern, logline string) []string {
	re := regexp.MustCompile(pattern)
	// a return value of nil indicates no match
	return re.FindStringSubmatch(logline)
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
			// extract time, message reason, decision, disrupted-node-count, replacment-node-count, podcount and nodeclaim
			patterns := []string{
				`"time":"(.*)","logger".*"reason":"(.*)","decision":"(.*)","disrupted-node-count":(.*),"replacement-node-count":(.*),"pod-count":(.*),"disrupted-nodes":.*,"NodeClaim":{"name":"(.*)"},"capacity-type"`,
				`"time":"(.*)","logger".*"command":"(.*)","decision":"(.*)","disrupted-node-count":(.*),"replacement-node-count":(.*),"pod-count":(.*),"disrupted-nodes":.*,"NodeClaim":{"name":"(.*)"},"capacity-type"`,
			}
			var matchslicesub []string
			var isCommandField bool
			for i, pattern := range patterns {
				if matchslicesub = matchPattern(pattern, logline); matchslicesub != nil {
					isCommandField = (i == 1)
					break
				}
			}
			if matchslicesub != nil {
				if nodeclaim = matchslicesub[7]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					entry.Disruptiontime = matchslicesub[1]
					if isCommandField {
						if idx := strings.IndexByte(matchslicesub[2], '/'); idx > 0 {
							entry.Disruptionreason = strings.ToLower(matchslicesub[2][:idx])
						}
					} else {
						entry.Disruptionreason = matchslicesub[2]
					}
					entry.Disruptiondecision = matchslicesub[3]
					entry.Disruptednodecount = matchslicesub[4]
					entry.Replacementnodecount = matchslicesub[5]
					entry.Disruptedpodcount = matchslicesub[6]
					(*nodeclaimmap)[nodeclaim] = entry
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
