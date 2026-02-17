use metrics::{counter, histogram};

pub fn record_verification(valid: bool, duration: f64, reason: Option<&str>){
    counter!("verifier_requests_total", 1);
    histogram!("verifier_request_duration_seconds", duration);

    if valid {
        counter!("verifier_signature_valid_total", 1);
    } else {
        let label = reason.unwrap_or("unknown").to_string();
        counter!(
            "verifier_signature_invalid_total",
            1,
            "reason" => label
        );
    }
}