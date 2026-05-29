use metrics::{counter, histogram};

pub fn record_verification(valid: bool, duration: f64, reason: Option<&str>) {
    histogram!("verifier_request_duration_seconds").record(duration);

    if valid {
        counter!("verifier_signature_valid_total").increment(1);
    } else {
        let label = reason.unwrap_or("unknown").to_string();
        counter!(
            "verifier_signature_invalid_total",
            "reason" => label
        )
        .increment(1);
    }
}
