
use reqwest::{header::{HeaderMap, HeaderValue, AUTHORIZATION, CONTENT_TYPE}, Error};
use serde::{Serialize, Deserialize};

#[derive(Serialize)]
struct Query {
    query: String,
    variables: Variables,
}

#[derive(Serialize)]
struct Variables {
    search: String,
}

#[derive(Deserialize, Debug)]
struct Media {
    id: i32,
    title: Title,
}

#[derive(Deserialize, Debug)]
struct Title {
    romaji: String,
    english: Option<String>,
    native: String,
}

#[derive(Deserialize, Debug)]
struct Page {
    media: Vec<Media>,
}

#[derive(Deserialize, Debug)]
struct ResponseData {
    Page: Page,
}

#[derive(Deserialize, Debug)]
struct ApiResponse {
    data: ResponseData,
}

pub struct AnimeApi {
    token: String,
}

impl AnimeApi {
    // Public constructor for the struct
    pub fn new(token: String) -> Self {
        AnimeApi { token }
    }

    pub fn search_anime_anilist(&self, query: &str) -> Result<ApiResponse, reqwest::Error> {
        let url = "https://graphql.anilist.co";

        let query_string = r#"
        query ($search: String) {
          Page(page: 1, perPage: 10) {
            media(search: $search, type: ANIME) {
              id
              title {
                romaji
                english
                native
              }
            }
          }
        }
        "#;

        let variables = Variables {
            search: query.to_string(),
        };

        let request_body = Query {
            query: query_string.to_string(),
            variables,
        };

        let mut headers = HeaderMap::new();
        headers.insert(AUTHORIZATION, HeaderValue::from_str(&format!("Bearer {}", self.token)).unwrap());
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));

        // Send the request
        let client = reqwest::blocking::Client::new();
        let response = client
            .post(url)
            .headers(headers)
            .json(&request_body)
            .send()?;

        // Check if the response is successful
        if response.status().is_success() {
            let api_response: ApiResponse = response.json()?;
            Ok(api_response)
        } else {
            Err(Error)
        }
    }
}


// pub fn parse_search_anime_anilist(response_text: &String) -> Result<Vec<SkipTime>, Box<dyn Error>>{
//     // Deserialize the JSON into AniskipResponse
//     let parsed_data: AniskipResponse = serde_json::from_str(response_text)?;

//     // If no skips are found, return an empty vector
//     if !parsed_data.found {
//         return Ok(Vec::new());
//     }
//     // Convert the parsed results into SkipTime structs
//     let skip_times: Vec<SkipTime> = parsed_data.results.into_iter().map(|result| {
//         SkipTime::new(
//             result.kind,
//             result.interval.start as i32,
//             result.interval.end as i32,
//         )
//     }).collect();

//         Ok(skip_times)
// }