use anyhow::Result;

/// Data-plane agent for multi-node deployments.
/// Future: runs on worker nodes, receives deploy artifacts from control plane.
#[derive(clap::Args)]
pub struct AgentArgs {
    /// Control plane URL
    #[arg(long)]
    pub control: String,

    /// This node's ID (assigned by `remo node add`)
    #[arg(long)]
    pub node_id: String,
}

pub async fn run(_args: AgentArgs) -> Result<()> {
    anyhow::bail!("multi-node agent not yet implemented")
}
