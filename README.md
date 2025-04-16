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

## Start using LogParserForKarpenter
[Amazon QuickSight](https://docs.aws.amazon.com/quicksight/latest/user/welcome.html) or Microsoft Excel are possible choices to use the CSV output to create graphs and/or pivot tables.

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

