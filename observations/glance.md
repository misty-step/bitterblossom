The `/observations` directory functions as the central feedback loop and telemetry repository for the `bitterblossom` project, specifically focused on monitoring and evaluating the performance of "sprites" (specialized AI agents). Its primary purpose is to capture qualitative and quantitative data regarding task execution, routing efficacy, and system-level friction to inform iterative configuration adjustments.

### Architecture and Key File Roles
The directory utilizes a tiered storage architecture that separates active, human-readable analysis from long-term, machine-readable persistence:

*   **`OBSERVATIONS.md`**: This file serves as the core operational journal and active feedback mechanism. It follows a structured schema to log individual task dispatches, including metadata such as sprite identity (e.g., Thorn, Bramble), task outcomes, routing justifications, and specific quality notes. It also contains an **Experiment Log** for tracking A/B tests and composition hypotheses (e.g., specialized sprites vs. generalists).
*   **`/archives`**: This subdirectory acts as a long-term data sink for the module. It is designed for data density and durability, housing immutable records of past observations and telemetry that are no longer required for active operations. Architecturally, it is organized hierarchically by date or ID to manage high volumes of serialized datasets (JSON, Parquet, CSV), compressed log bundles, and metadata manifests.

### Dependencies and Technical Gotchas
The directory relies on lifecycle management scripts for data rotation and filesystem-level compression libraries to manage the growth of the `/archives` subdirectory. Key technical considerations include:

*   **Schema Drift**: As the `bitterblossom` data models evolve, historical records in the `/archives` directory may become incompatible with current parsers or analysis tools.
*   **Resource Management**: The archive is susceptible to unmanaged disk exhaustion if automated purging or off-site migration policies are not strictly enforced.
*   **Observability Gaps**: Current logging processes may experience buffering issues where output is not visible until task completion, complicating real-time monitoring.
*   **Environment Fragility**: Observations indicate that sprite operations are highly dependent on local environment configurations, such as the presence of `GH_TOKEN` or specific CLI tool versions (e.g., `gh` API changes), which can lead to execution hangs or failures documented within the journal.