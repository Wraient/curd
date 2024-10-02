// use std::process::exit;
use curd::aniskip::{get_skip_times, SkipType, SkipTime};
use curd::Anime;
// mod anilist;

fn main() {
    // let query = "Naruto";
    // let token = "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsImp0aSI6IjY5NTU3NWExMzY5NjE5ZWUxZWYwNTY5OTA0MDUxZDc0NTE5ZTM2MmZmZDViOTI5YjE1YjMwMTBjZWQyNDMyZDJiYmZlNTcxZmMxNGU0OGQ0In0.eyJhdWQiOiIyMDY4NiIsImp0aSI6IjY5NTU3NWExMzY5NjE5ZWUxZWYwNTY5OTA0MDUxZDc0NTE5ZTM2MmZmZDViOTI5YjE1YjMwMTBjZWQyNDMyZDJiYmZlNTcxZmMxNGU0OGQ0IiwiaWF0IjoxNzI3MjcyNDkxLCJuYmYiOjE3MjcyNzI0OTEsImV4cCI6MTc1ODgwODQ5MSwic3ViIjoiNjcxOTUyNiIsInNjb3BlcyI6W119.cpypIpuRu1D0LfNZpKgNVyGGViEVeaB9IZ2zkWcoudfzmrT8xIweYgpCLjK7pCliPFX4UhT3oC8vYh1kwDmUKGkr6TcyJ4zWEDpRVZUZwtT6jt6y9cotNu5Wifk2FRSKQ4R9zLTsrgFRiSxHQC5gMk6-qbVsv94AO77QNIRXNTcjGY0260S-EQFsm988s77yEsWiGL27OJCU6D02ik89mS0vQqdifh3HvnJQI7rJOctg0zRouHSbhj8csevjzOcdU2zjwO669rn9zZZUerA99Sb1fUEQuAt-c7A8TdAJwekdX50bPNOETs-pHxH4maOW_5yV-osTVvdC8WoTSWr8rmGN3pvxhGJ-Z97nCw5L-RCyoJV93DkGZie4esfUPEKoewzB4l6TUtbuUFT0EYF-QD4Wh7dpzBcMXiVJUVyhfEuf25LpFknn3r3L2ATPj9MM-tgGLJQCqHKgEn0MVys6vbZJmYxPNAnQ9wej0hrLno1JoP8ap7obnU8_BG3cx9PDzgRHT_X6goH4V74tgxspt6gIUblDJC4xNjodnKbgrz3CCHNhb_HVWY7OtsdenBn69nNNz1iN4ZzH2ioksYzpfEm0tVoHmzIxGNDRJMe5yPKYO_ccOTviez7uIY4r2w2t9roNt-4pT5UkevKr-vrTKlx_LtlWl42u8KXfv1ta9fY"; // Replace with your token

    // match anilist::search_anime_anilist(query, token) {
    //     Ok(response) => {
    //         println!("{:#?}", response);
    //     }
    //     Err(err) => {
    //         eprintln!("Error: {:?}", err);
    //     }
    // }

    let mut anime = Anime::new();
    anime.anilist_id(21);
    anime.watching_ep(2);
    // let episode_no = 2;
    anime.skip_times(
        match get_skip_times(anime.anilist_id.unwrap(), anime.watching_ep.unwrap()) {
            Ok(skips) => {
                skips
            },
            Err(_) => {
                println!("Failed to get skip times");
                vec![SkipTime::new(SkipType::Op, 0, 0), SkipTime::new(SkipType::Ed, 0, 0)]
            }
        }
    );

    anime.skip_times.map(|anime_skip| { // Would never fail as it would be 0 0
        for skip in anime_skip {
            if skip.kind == SkipType::Op {
                println!("Opening: {} to {}", skip.intervals.start, skip.intervals.end);
            }
            else if skip.kind == SkipType::Ed {
                println!("Ending: {} to {}", skip.intervals.start, skip.intervals.end);
            }
        }
    });

}