# Security Policy

## Supported versions

Security fixes are provided for the latest released version.

## Reporting a vulnerability

Please do not open a public issue for security-sensitive reports.

Report vulnerabilities by contacting the maintainer through GitHub or by creating a private security advisory if the repository has advisories enabled.

When reporting, include:

- A clear description of the issue.
- A minimal reproduction if possible.
- Impact and affected versions.
- Any relevant logs or traces with secrets removed.

## Security considerations

- Do not put raw tokens, passwords, phone numbers, emails, or private data directly in rate limit keys.
- Hash sensitive identifiers before using them as keys.
- Treat Redis errors explicitly in production, for example by falling back to memory mode or rejecting requests.
- Memory fallback does not share state across instances and should only be used as a temporary safety net.
