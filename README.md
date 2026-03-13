# Log Parser for Karpenter (LogParserForKarpenter)

<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" width="100">  <img src="https://github.com/aws/karpenter-provider-aws/blob/main/website/static/banner.png" width="200">
----

**Log Parser for Karpenter** is a Golang based command line tool which can parse the output of Karpenter Controller logs, tested against logs from Karpenter versions:
* v0.37.7 \*
* v1.0.x \*
* v1.1.x
* v1.2.x
* v1.3.x
* v1.4.x
* v1.9.0

\* Note: `"messageKind":"spot_interrupted"` is first supported with Karpenter version v1.1.x, so **LogParserForKarpenter (lp4k)** does not provide *interruptiontime* and *interruptionkind* in earlier versions

It allows using either STDIN (for example for piping live Karpenter controller logs) or multiple Karpenter log files as input and will print CSV style formatted output of nodeclaim data ordered by createdtime to STDOUT, so one can easily redirect it into a file and analyse with tools like [Amazon QuickSight](https://docs.aws.amazon.com/quicksight/latest/user/welcome.html) or Microsoft Excel.

If neither STDIN nor log files are used as input, **lp4k** will attach to a running K8s/EKS cluster and parses Karpenter logs (streamed logs, similar to *kubectl logs -f* using LP4K_KARPENTER_NAMESPACE and LP4K_KARPENTER_LABEL) and creates a ConfigMap *lp4k-cm-\<date\>* in same namespace, which gets updated every LP4K_CM_UPDATE_FREQ.

K8s handling can be configured using the following OS environment variables:

| Environment variable      | Default value     | Description
| ------------- | ------------- | ------------- |
| LP4K_KARPENTER_NAMESPACE | "kube-system" | K8s namespace where Karpenter controller is running
| LP4K_KARPENTER_LABEL | "app.kubernetes.io/name=karpenter" | Karpenter controller K8s pod labels
| LP4K_CM_UPDATE_FREQ | "30s" | update frequency of ConfigMap and STDOUT if enabled (default), must be valid Go time.Duration string like "30s" or 2m30s"
| LP4K_CM_PREFIX | "lp4k-cm" | nodeclaim ConfigMap prefix, if KARPENTER_LP4K_CM_OVERRIDE=false or ConfigMap name, if KARPENTER_LP4K_CM_OVERRIDE=true
| LP4K_CM_OVERRIDE | "false" | determines, if ConfigMap will just use prefix and will be overriden upon every start of lp4k
| LP4K_NODECLAIM_PRINT | "true" | print nodeclaim information every KARPENTER_CM_UPDATE_FREQ to STDOUT
| LP4K_TIME_FORMAT | "2006-01-02-15-04-05" | time format for ConfigMap names and S3 object timestamps, must be a valid Go time layout string

\* Note: In mode `LP4K_CM_OVERRIDE=true` **lp4k** will read existing nodeclaim data from ConfigMap specified by LP4K_CM_PREFIX

### S3 Upload Configuration

**lp4k** can automatically upload parsed Karpenter log data to Amazon S3. This feature is optional and only enabled when the S3 bucket environment variable is set.

| Environment variable      | Default value     | Description
| ------------- | ------------- | ------------- |
| LP4K_S3_BUCKET | "" (disabled) | S3 bucket name where CSV files will be uploaded. S3 upload is only enabled when this is set
| LP4K_S3_PREFIX | "karpenter-logs" | S3 key prefix for uploaded files
| LP4K_S3_REGION | "us-east-1" | AWS region for S3 bucket
| LP4K_S3_OVERWRITE | "false" | If true, overwrites the same S3 object (using program start time) on each update. If false, creates new timestamped objects on each update

When S3 upload is enabled, **lp4k** will:
- Upload CSV files with timestamp in the filename: `karpenter-nodeclaims-YYYY-MM-DD-HH-MM-SS.csv`
- Upload after parsing completes (file mode) or periodically during streaming (K8s mode, every LP4K_CM_UPDATE_FREQ)
- Use AWS SDK default credential chain (IAM roles, environment variables, AWS config files, etc.)

Example usage:
```bash
# Enable S3 upload
export LP4K_S3_BUCKET=my-karpenter-logs-bucket
export LP4K_S3_PREFIX=production/karpenter-logs
export LP4K_S3_REGION=us-west-2

# Run lp4k with file input
./bin/lp4k karpenter-logs.txt

# Or run in K8s streaming mode
./bin/lp4k
```

**AWS Credentials:** Ensure your AWS credentials are configured. The tool uses the standard AWS SDK credential chain:
- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
- AWS credentials file (`~/.aws/credentials`)
- IAM role (when running on EC2/EKS)
- EKS Pod Identity or IRSA (when running as a pod in EKS)

**IAM Permissions Required:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::your-bucket-name/*"
    }
  ]
}
```

----

Use:
```bash
LP4K_CM_UPDATE_FREQ=10s ./bin/lp4k
```
or permanently
```bash
export LP4K_CM_UPDATE_FREQ=10s
./bin/lp4k
```
----

## To start using LogParserForKarpenter

Just run:
```bash
make
```
A binary `lp4k` for your OS and platform is build in directory `bin`.

Then run it like:
```bash
./bin/lp4k sample-input.txt
```
or
```bash
./bin/lp4k <Karpenter log output file 1> [... <Karpenter log output file n>]
```
or
```bash
kubectl logs -n kube-system <Karpenter leader pod> [-f] | ./lp4k
```
or for attaching to K8s/EKS cluster in current KUBECONFIG context
```bash
./bin/lp4k
```
The sample output file [sample-multi-file-klp-output.csv](sample-multi-file-klp-output.csv) shows all exposed nodeclaim information and can be used as a sample starter to build analysis on top of it.
* Note: **lp4k** will recognise new nodeclaims and populate its internal structures first when Karpenter controller logs show a logline containing `"message":"created nodeclaim"`. That means after a Karpenter controller restart and a subsequent and required restart **lp4k** will not recognise already existing nodeclaims and shows `No results - empty "nodeclaim" map`

### lp4kcm

**lp4kcm** is a helper tool to display **lp4k** ConfigMap data in same CSV format.

Just run:
```bash
make tools
```
A binary `lp4kcm` for your OS and platform is build in directory `bin`.

Then run it like:
```bash
./bin/lp4k <lp4k ConfigMap name 1> [... <lp4k ConfigMap name n>]
```

## Analyse LogParserForKarpenter output
The simplest way for analysis is to use the output and parse it using standard Linux utilities like awk, cut and grep.
```console
# indexed header
$ head -1 sample-multi-file-klp-output.csv 
nodeclaim[1],createdtime[2],nodepool[3],instancetypes[4],launchedtime[5],providerid[6],instancetype[7],zone[8],capacitytype[9],registeredtime[10],k8snodename[11],initializedtime[12],nodereadytime[13],nodereadytimesec[14],disruptiontime[15],disruptionreason[16],disruptiondecision[17],disruptednodecount[18],replacementnodecount[19],disruptedpodcount[20],annotationtime[21],annotation[22],tainttime[23],taint[24],interruptiontime[25],interruptionkind[26],deletedtime[27],nodeterminationtime[28],nodeterminationtimesec[29],nodelifecycletime[30],nodelifecycletimesec[31],initialized[32],deleted[33]

# print nodeclaim[index/column=1], nodereadytime[13],nodereadytimesec[14]
$ cat sample-multi-file-klp-output.csv | awk -F  ',' '{print $1,$13,$14 }' | more
nodeclaim[1] nodereadytime[13] nodereadytimesec[14]
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
$ ./bin/lp4k karpenter-log-0.37.7.txt | awk -F  ',' '{ print $1,$13,$14 }'
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

## Contributing

We welcome contributions to **lp4k** ! Please see [CONTRIBUTING.md](doc/CONTRIBUTING.md) for more information on how to report bugs or submit pull requests.

### Code of conduct

This project has adopted the [Amazon Open Source Code of Conduct](https://aws.github.io/code-of-conduct). See [CODE_OF_CONDUCT.md](doc/CODE_OF_CONDUCT.md) for more details.

### Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache 2.0 License.
