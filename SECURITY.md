# Security Policy

Security has been Konfirm's top priority since its inception, and we remain committed to maintaining an uncompromisingly secure solution for multi-tenanted production environments.

## Security Commitments

The Konfirm project is committed to the following:
 - Project dependencies will be kept up to date for all supported versions. Unsupported or end-of-life dependencies will be removed.
 - Kubernetes Namespace isolation will be respected. Users will not be able to infer information across namespaces unless the source namespace is explicitly configured to permit it (e.g., with a HelmPolicy).
 - Official Helm charts will be secure by default and adhere to the principle of least privilege. 

Any application behavior that is not consistent with these objectives is considered a BUG and will be addressed by the project maintainers.

## Reporting Vulnerabilities and Other Security Issues

**Please DO NOT report vulnerabilities via GitHub issues.**

We appreciate any efforts from the community to help secure Konfirm, and respectfully request researchers work with the Konfirm maintainers to ensure a responsible, coordinated disclosure.

Please report vulnerabilities or other security issues via email, to [dhenderson@teamraft.com](mailto:dhenderson@teamraft.com?subject=Konfirm%20Security%20Issue). You should receive a response within 48 hours. If you do not, please send an additional message.

We ask that your report include as much detail as possible to aid in rapid confirmation and response, including:
 - the type of vulnerability (e.g., information disclosure, code execution, etc.)
 - detailed steps for reproducing the vulnerability
 - if available, sample code demonstrating the vulnerability

We prefer submissions in English.