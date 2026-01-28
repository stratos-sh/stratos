## Requirements

### Requirement: AWSNodeClass supports IMDS metadata options

AWSNodeClass SHALL support a `metadataOptions` field that configures the EC2 Instance Metadata Service for launched instances.

The `metadataOptions` field SHALL support:
- `httpTokens`: "required" (IMDSv2 only) or "optional" (IMDSv1 and v2). Defaults to EC2 default if not specified.
- `httpPutResponseHopLimit`: Integer 1-64. Controls the HTTP PUT response hop limit for IMDS token requests.
- `httpEndpoint`: "enabled" or "disabled". Controls whether the IMDS endpoint is available.

#### Scenario: IMDSv2 enforced
- **WHEN** an AWSNodeClass has `metadataOptions.httpTokens: "required"`
- **THEN** launched instances SHALL have IMDS configured to require session tokens (IMDSv2)

#### Scenario: Custom hop limit
- **WHEN** an AWSNodeClass has `metadataOptions.httpPutResponseHopLimit: 2`
- **THEN** launched instances SHALL have the IMDS hop limit set to 2

#### Scenario: Metadata options not specified
- **WHEN** an AWSNodeClass does not have a `metadataOptions` field
- **THEN** launched instances SHALL use EC2 default metadata options

#### Scenario: IMDS disabled
- **WHEN** an AWSNodeClass has `metadataOptions.httpEndpoint: "disabled"`
- **THEN** launched instances SHALL have the IMDS endpoint disabled

### Requirement: Metadata options are passed through at launch time

The AWS provider SHALL pass `metadataOptions` to the EC2 `RunInstances` API as `MetadataOptions` on the launch request. No resolution or status caching is needed for this field.

#### Scenario: Launch includes metadata options
- **WHEN** the AWS provider launches an instance for an AWSNodeClass with `metadataOptions` set
- **THEN** the `RunInstances` API call SHALL include the `MetadataOptions` parameter with the specified values
