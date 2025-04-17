# Log Parser for Karpenter (LogParserForKarpenter)

<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" width="100">  <img src="https://github.com/aws/karpenter-provider-aws/blob/main/website/static/banner.png" width="200">
----

**Log Parser for Karpenter** is a Golang based command line tool which can parse the output of Karpenter Controller logs, tested against logs from Karpenter versions:
* v0.37.7 \*
* v1.0.x \*
* v1.1.x
* v1.2.x
* v1.3.x

\* Note: "messageKind":"spot_interrupted" is first supported with Karpenter version v1.1.x, so **LogParserForKarpenter (lp4k)** does not provide *interruptiontime* and *interruptionkind* in earlier versions

It allows using either STDIN (for example for piping live Karpenter controller logs) or multiple Karpenter log files as input and will print CSV style formatted output of nodeclaim data ordered by createdtime to STDOUT, so one can easily redirect it into a file and analyse with tools like [Amazon QuickSight](https://docs.aws.amazon.com/quicksight/latest/user/welcome.html) or Microsoft Excel.

If neither STDIN nor log files are used as input, **lp4k** will attach to a running K8s/EKS cluster and parses Karpenter logs (streamed logs, similar to *kubectl logs -f*) and creates a ConfigMap *karpenter-nodeclaims-cm* in *karpenter* namespace, which gets updated every 30s.

\* Note: K8s handling currently requires **Karpenter** running in namespace *karpenter* and is not able to re-attach to **Karpenter** pods after restarts (re-deployment etc.)

----

## To start using LogParserForKarpenter

Just run:
```bash
make
```
A binary `lp4k` for your OS and platform is build in directory `bin`.

Then run it like:
```bash
./lp4k <Karpenter log output file 1> [... <Karpenter log output file n>]
```
or
```bash
kubectl logs -n karpenter -l=app.kubernetes.io/name=karpenter [-f] | ./lp4k
```
or for attaching to K8s/EKS cluster in current KUBECONFOG context
```bash
./lp4k
```

The sample output file [sample-multi-file-klp-output.csv](sample-multi-file-klp-output.csv) shows all exposed nodeclaim information and can be used as a sample starter to build analysis on top of it.

## Analyse LogParserForKarpenter output
The simplest way for analysis is to use the output and parse it using standard Linux utilities like awk, cut and grep.
```console
# indexed header
$ head -1 sample-multi-file-klp-output.csv 
nodeclaim(1),createdtime(2),nodepool(3),instancetypes(4),launchedtime(5),providerid(6),instancetype(7),zone(8),capacitytype(9),registeredtime(10),k8snodename(11),initializedtime(12),nodereadytime(13),nodereadytimesec(14),disruptiontime(15),disruptionreason(16),disruptiondecision(17),disruptednodecount(18),replacementnodecount(19),disruptedpodcount(20),annotationtime(21),annotation(22),tainttime(23),taint(24),interruptiontime(25),interruptionkind(26),deletedtime(27),nodeterminationtime(28),nodeterminationtimesec(29),nodelifecycletime(30),nodelifecycletimesec(31),initialized(32),deleted(33)

# print nodeclaim(index/column=1), nodereadytime(13),nodereadytimesec(14)
$ cat sample-multi-file-klp-output.csv | awk -F  ',' '{print $1,$13,$14 }' | more
nodeclaim(1) nodereadytime(13) nodereadytimesec(14)
spot-844xp 1m18.591s 78.6
default-brbk4 0s 0.0
default-lpc62 50.935s 50.9
default-j4lj7 43.617s 43.6
default-mpz2w 46.277s 46.3
default-8sxj9 36.714s 36.7
default-zgb22 35.008s 35.0
local-storage-raid-al2023-kdsvk 41.839s 41.8
local-storage-raid-al2023-tq7v5 43.922s 43.9
local-storage-raid-al2023-9kx8z 48.781s 48.8
...

# search for specific value sof a specific nodeclaim
$ cat sample-multi-file-klp-output.csv | awk -F  ',' '/local-storage-raid-al2023-nccxt/ { print $1,$13,$14 }'
local-storage-raid-al2023-nccxt 1m8.447s 68.4

# combine directly with lp4k and raw Karpenter controller logs
$ ./lp4k karpenter-log-0.37.7.txt | awk -F  ',' '{ print $1,$13,$14 }'
nodeclaim(1) nodereadytime(13) nodereadytimesec(14)
default-brbk4 0s 0.0
default-lpc62 50.935s 50.9
default-j4lj7 43.617s 43.6
default-mpz2w 46.277s 46.3
```
[Amazon QuickSight](https://docs.aws.amazon.com/quicksight/latest/user/welcome.html) or Microsoft Excel are possible choices to use the CSV output for advanced analysis to create graphs and/or pivot tables.

![Sample 1](Quicksight_sample_graph.png
 "Sample Quicksight graph")
![Sample 2](Quicksight_sample_pivot_table.png
 "Sample Quicksight pivot table")

## To start developing LogParserForKarpenter

Please contact (mailto:waltju@amazon.com)

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache 2.0 License.

