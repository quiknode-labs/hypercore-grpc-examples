fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure().compile(
        &["../proto/hyperliquid.proto", "../proto/orderbook.proto"],
        &["../proto"],
    )?;
    Ok(())
}
