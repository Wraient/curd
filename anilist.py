import requests
import json


import requests

def get_anilist_user_id(token):
    url = "https://graphql.anilist.co"
    query = '''
    query {
        Viewer {
            id
            name
        }
    }
    '''
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "application/json"
    }
    
    response = requests.post(url, json={"query": query}, headers=headers)
    
    if response.status_code == 200:
        data = response.json()
        user_id = data['data']['Viewer']['id']
        user_name = data['data']['Viewer']['name']
        return user_id, user_name
    else:
        raise Exception(f"Error: {response.status_code}, {response.text}")

# Replace 'your_token' with your actual AniList token


def get_user_data(access_token, user_id):
# Replace with your actual access token and user ID
  # access_token = 
  # user_id = 

  # GraphQL query
  query = """
  {
    MediaListCollection(userId: %s, type: ANIME) {
      lists {
        entries {
          media {
            id
            episodes
            duration
            title {
              romaji
              english
            }
          }
          status
          score
          progress
        }
      }
    }
  }
  """ % user_id

  # Send the request
  response = requests.post(
      'https://graphql.anilist.co',
      json={'query': query},
      headers={'Authorization': f'Bearer {access_token}'}
  )


  # Print the response
  return response.json()

def load_json_file(file_path):
    with open(file_path, 'r', encoding='utf-8') as file:
        return json.load(file)

def search_anime_by_title(json_data, search_title):
    results = []
    if search_title=="1P":
      search_title = "ONE PIECE"
    for list_item in json_data['data']['MediaListCollection']['lists']:
        for entry in list_item['entries']:
            media = entry['media']
            romaji_title = media['title']['romaji']
            english_title = media['title']['english']
            episodes = media['episodes']
            duration = media['duration']
            try:
              if search_title.lower() in romaji_title.lower() or search_title.lower() in english_title.lower():
                  results.append({
                      'id': media['id'],
                      'progress': entry['progress'],
                      'romaji_title': romaji_title,
                      'english_title': english_title,
                      'episodes': episodes,
                      'duration': duration,
                  })
            except:
              pass
    
    return results

def update_anime_progress(token: str, media_id: int, progress: int):
    url = "https://graphql.anilist.co"
    
    query = '''
    mutation($mediaId: Int, $progress: Int) {
        SaveMediaListEntry(mediaId: $mediaId, progress: $progress) {
            id
            progress
        }
    }
    '''
    
    variables = {
        "mediaId": media_id,  # The AniList ID of the anime
        "progress": progress  # The number of the latest episode you watched
    }
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "application/json"
    }
    
    response = requests.post(url, json={"query": query, "variables": variables}, headers=headers)
    
    if response.status_code == 200:
        data = response.json()
        updated_progress = data['data']['SaveMediaListEntry']['progress']
        print("updating progress..")
        print(f"Anime progress updated! Latest watched episode: {updated_progress}")
    else:
        print(f"Error {response.status_code}: {response.text}")

def main():
    # Load the JSON file
    json_file_path = 'response.json'  # Replace with the path to your JSON file
    json_data = load_json_file(json_file_path)
    
    # Ask for the title to search
    # search_title = input("Enter the anime title to search (Romaji or English): ")

    search_title = "1P"
    
    # Search for the anime
    results = search_anime_by_title(json_data, search_title)
    
    # Print the results
    if results:
        for result in results:
            print(f"Anime ID: {result['id']}, Progress: {result['progress']}, "
                  f"Romaji Title: {result['romaji_title']}, English Title: {result['english_title']}")
    else:
        print("Anime not found.")

if __name__ == "__main__":
    main()


# if __name__ == '__main__':
#   # Load JSON data from a file (replace 'your_file.json' with the actual file name)
#   with open('response.json', 'r') as file:
#       data = json.load(file)

#   # Define the title to search for
#   search_title = 'Fullmetal Alchemist: Brotherhood'  # Replace with the actual title you're searching for

#   # Search for the anime and get its ID
#   anime_id = search_anime_by_title(data, search_title)

#   # Print the result
#   if anime_id:
#       print(f"Anime ID: {anime_id}")
#   else:
#       print("Anime not found.")
