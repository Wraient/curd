use reqwest::{blocking, Response};
use serde::{Deserialize, Serialize};
use std::{error::Error};
use serde_json;

// Define the enum for SkipType
#[derive(Debug, Deserialize)]
pub enum SkipType {
    #[serde(rename = "op")]
    Op,
    #[serde(rename = "ed")]
    Ed,
}

// Define the struct for the interval part
#[derive(Debug, Deserialize)]
pub struct Interval {
    #[serde(rename = "start_time")]
    pub start: f64,
    #[serde(rename = "end_time")]
    pub end: f64,
}

// Define the struct for the response result
#[derive(Debug, Deserialize)]
pub struct SkipResult {
    pub interval: Interval,
    #[serde(rename = "skip_type")]
    pub kind: SkipType,
    #[serde(rename = "episode_length")]
    pub episode_length: f64,
}

// Define the struct for the entire response
#[derive(Debug, Deserialize)]
pub struct AniskipResponse {
    pub found: bool,
    pub results: Vec<SkipResult>,
}

// Struct to represent the simplified skip time
#[derive(Debug)]
pub struct SkipTime {
    pub kind: SkipType,
    pub intervals: SkipInterval,
}

// Interval struct for simplified handling
#[derive(Debug)]
pub struct SkipInterval {
    pub start: i32,
    pub end: i32,
}

impl SkipTime {
    // Constructor for SkipTime
    pub fn new(kind: SkipType, start: i32, end: i32) -> Self {
        SkipTime {
            kind,
            intervals: SkipInterval { start, end },
        }
    }
}

// Function to get aniskip data
pub fn get_aniskip_data(anime_id: i32, episode_number: i32) -> Result<String, Box<dyn Error>> {
    let base_url = "https://api.aniskip.com/v1/skip-times";
    let url = format!("{base_url}/{anime_id}/{episode_number}?types=op&types=ed"); 

    let response = blocking::get(url)?;
    // Check if the request was successful
    if response.status().is_success() {
        // Return the response body
        let body = response.text()?;
        Ok(body)
    } else {
        Err(format!("Failed to retrieve the page. Status: {}", response.status()).into())
    }
}

// Function to parse the Aniskip response and return SkipTime structs
pub fn parse_aniskip_response(response_text: &String) -> Result<Vec<SkipTime>, Box<dyn Error>> {
    // Deserialize the JSON into AniskipResponse
    let parsed_data: AniskipResponse = serde_json::from_str(response_text)?;

    // If no skips are found, return an empty vector
    if !parsed_data.found {
        return Ok(Vec::new());
    }

    // Convert the parsed results into SkipTime structs
    let skip_times: Vec<SkipTime> = parsed_data.results.into_iter().map(|result| {
        SkipTime::new(
            result.kind,
            result.interval.start as i32,
            result.interval.end as i32,
        )
    }).collect();

    Ok(skip_times)
}

// Unit tests
#[cfg(test)]
mod tests {
    use super::*; // Import the functions and types from the current module

    #[test]
    fn test_skip_time_creation() {
        let skip = SkipTime::new(SkipType::Op, 10, 30);

        match skip.kind {
            SkipType::Op => assert_eq!(skip.intervals.start, 10),
            _ => panic!("Expected SkipType::Op"),
        }
    }

    #[test]
    fn test_skip_time_ed_creation() {
        let skip = SkipTime::new(SkipType::Ed, 1100, 1130);

        match skip.kind {
            SkipType::Ed => {
                assert_eq!(skip.intervals.start, 1100);
                assert_eq!(skip.intervals.end, 1130);
            }
            _ => panic!("Expected SkipType::Ed"),
        }
    }

    #[test]
    fn tmp(){
        // let skip = SkipTime::new(SkipType::Ed, 1100, 1130);
        // println!("{}", skip);
    }

}


