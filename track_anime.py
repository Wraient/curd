import csv

# Path to your text database file
# database_file = 'curd_history.txt'

# Function to add a new anime entry
def add_anime(database_file, anilist_id, allanime_id, episode, time, duration, name):
    with open(database_file, mode='a', newline='') as file:
        writer = csv.writer(file)
        writer.writerow([anilist_id, allanime_id, episode, time, duration, name])
    print("Written to file")

# Function to delete an anime entry by Anilist ID and Allanime ID
def delete_anime(database_file, anilist_id, allanime_id):
    anime_list = []
    with open(database_file, mode='r', newline='') as file:
        reader = csv.reader(file)
        for row in reader:
            # Keep only entries that don't match the Anilist ID and Allanime ID
            if row[0] != anilist_id or row[1] != allanime_id:
                anime_list.append(row)

    # Rewrite the database without the deleted entry
    with open(database_file, mode='w', newline='') as file:
        writer = csv.writer(file)
        writer.writerows(anime_list)

# Function to retrieve all anime entries
def get_all_anime(database_file):
    anime_list = []
    with open(database_file, mode='r', newline='') as file:
        reader = csv.reader(file)
        for row in reader:
            anime_list.append({
                'anilist_id': row[0],
                'allanime_id': row[1],
                'episode': row[2],
                'time': row[3],
                'duration': row[4],
                'name': row[5]
            })
    return anime_list

# Function to update or add a new anime entry
def update_anime(database_file: str, anilist_id:str, allanime_id: str, episode:str, time:str, duration:str, name:str):
    anime_list = get_all_anime(database_file)
    updated = False

    # Create a new list to store updated entries
    updated_anime_list = []
    
    # Check if the anime entry exists and update it
    for anime in anime_list:
        if anime['anilist_id'] == anilist_id and anime['allanime_id'] == allanime_id:
            updated_anime_list.append({
                'anilist_id': anilist_id,
                'allanime_id': allanime_id,
                'episode': episode,
                'time': time,
                'duration': duration,
                'name': name
            })
            updated = True
        else:
            updated_anime_list.append(anime)
    
    # If no entry was updated, add new entry
    if not updated:
        updated_anime_list.append({
            'anilist_id': anilist_id,
            'allanime_id': allanime_id,
            'episode': episode,
            'time': time,
            'duration': duration,
            'name': name
        })

    # Write all entries back to the file
    with open(database_file, mode='w', newline='') as file:
        writer = csv.writer(file)
        for anime in updated_anime_list:
            writer.writerow([anime['anilist_id'], anime['allanime_id'], anime['episode'], anime['time'], anime['duration'], anime['name']])
    
    print("Updated the file")

def find_anime(anime_list, anilist_id=-1, allanime_id=-1):
    for anime in anime_list:
        if anime['anilist_id'] == anilist_id or anime['allanime_id'] == allanime_id:
            return anime
        
    return False

# Example usage
database_name = "/home/wraient/.local/share/curd/curd_history.txt"
# add_anime(database_name, '54321', '09876', '3', '100', '24',  'My Hero Academia')

# # Print all anime entries
# update_anime(database_name, '21', 'ReooPAxPMsHM4KPMY', '1110', '10', 'ONE PIECE')
# print(find_anime(get_all_anime(database_name), allanime_id="21"))
# update_anime(database_name, '21', 'ReooPAxPMsHM4KPMY', '1110', '101', 'ONE PIECE')

# # Update an entry
# update_anime(database_name, '12345', '67890', '13', '120', 'Attack on Titan')

# # Print all anime entries after update
print(get_all_anime(database_name))
