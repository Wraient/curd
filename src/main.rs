use anilist::AnimeApi;
// use std::process::exit;
use curd::aniskip::{get_skip_times, SkipType, SkipTime};
use curd::Anime;
mod anilist;

fn main() {
    let query = AnimeApi::new(String::from(""));
    let response = query.search_anime_anilist("One piece");
    println!("{:?}", response);
    // match anilist::AnimeApi::search_anime_anilist(query, ) {
    //     Ok(response) => {
    //         println!("{:#?}", response);
    //     }
    //     Err(err) => {
    //         eprintln!("Error: {:?}", err);
    //     }
    // }

    // let mut anime = Anime::new();
    // anime.anilist_id(21);
    // anime.watching_ep(2);
    // // let episode_no = 2;
    // anime.skip_times(
    //     match get_skip_times(anime.anilist_id.unwrap(), anime.watching_ep.unwrap()) {
    //         Ok(skips) => {
    //             skips
    //         },
    //         Err(_) => {
    //             println!("Failed to get skip times");
    //             vec![SkipTime::new(SkipType::Op, 0, 0), SkipTime::new(SkipType::Ed, 0, 0)]
    //         }
    //     }
    // );

    // anime.skip_times.map(|anime_skip| { // Would never fail as it would be 0 0
    //     for skip in anime_skip {
    //         if skip.kind == SkipType::Op {
    //             println!("Opening: {} to {}", skip.intervals.start, skip.intervals.end);
    //         }
    //         else if skip.kind == SkipType::Ed {
    //             println!("Ending: {} to {}", skip.intervals.start, skip.intervals.end);
    //         }
    //     }
    // });

}