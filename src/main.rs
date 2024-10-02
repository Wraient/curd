use std::process::exit;
use curd::aniskip::{get_aniskip_data, parse_aniskip_response, SkipInterval, SkipType, SkipResult, SkipTime};

fn main() {
    let anime_id = 21;
    let episode_no = 1;

    let mut response_text = get_aniskip_data(anime_id, episode_no);

    match response_text{
        Ok(response_text) => {
            let skip_times = parse_aniskip_response(&response_text);
            // println!("{:?}", skip_times);
            match skip_times{
                Ok(skip_times) => {
                    for skip in skip_times {
                        println!("{:?} skips from {} to {}", skip.kind, skip.intervals.start, skip.intervals.end);
                    }
                }
                Err(e) => {
                    eprintln!("Error: {e}")
                }
            }
        },
        Err(e) => {
            eprintln!("Error fetching Aniskip data: {}", e);
        }
    }


    // Ok(())

    // let number: f32 = 35.303;
    // println!("{}", number.round());
}