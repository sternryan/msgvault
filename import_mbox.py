import mailbox
import sqlite3
import hashlib
import os
import sys
import datetime
import email.utils
import json
from email.header import decode_header

def decode_str(s):
    if s is None:
        return ""
    decoded_list = decode_header(s)
    result = ""
    for b, enc in decoded_list:
        if isinstance(b, bytes):
            if enc:
                try:
                    result += b.decode(enc)
                except:
                    result += b.decode('utf-8', 'ignore')
            else:
                result += b.decode('utf-8', 'ignore')
        else:
            result += str(b)
    return result

def get_participants(message):
    parts = []
    for header in ['From', 'To', 'Cc']:
        val = message.get(header)
        if val:
            # Naive parsing, should use email.utils.getaddresses
            addrs = email.utils.getaddresses([val])
            for name, addr in addrs:
                parts.append({'name': decode_str(name), 'email': addr, 'type': header.lower()})
    return parts

def import_mbox(mbox_path, db_path):
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    # 1. Create Source
    source_id = "mbox_" + os.path.basename(mbox_path)
    cursor.execute("""
        INSERT OR IGNORE INTO sources (source_type, identifier, display_name) 
        VALUES ('gmail', ?, ?)
    """, (source_id, "MBOX Import: " + os.path.basename(mbox_path)))
    
    cursor.execute("SELECT id FROM sources WHERE identifier = ?", (source_id,))
    row = cursor.fetchone()
    if not row:
        print("Failed to create source")
        return
    db_source_id = row[0]
    
    mbox = mailbox.mbox(mbox_path)
    print(f"Importing {len(mbox)} messages from {mbox_path}...")
    
    for i, message in enumerate(mbox):
        try:
            msg_id = message.get('Message-ID', '').strip()
            if not msg_id:
                msg_id = hashlib.sha256(message.as_bytes()).hexdigest()
            
            subject = decode_str(message.get('Subject', ''))
            date_str = message.get('Date')
            date_ts = None
            if date_str:
                try:
                    date_ts = email.utils.parsedate_to_datetime(date_str)
                except:
                    pass
            
            # Conversation (Naive grouping by Subject for now, or X-GM-THRID)
            thread_id = message.get('X-GM-THRID')
            if not thread_id:
                # Fallback to subject hash
                thread_id = hashlib.md5(subject.encode('utf-8')).hexdigest()
            
            cursor.execute("""
                INSERT OR IGNORE INTO conversations (source_id, source_conversation_id, conversation_type, title)
                VALUES (?, ?, 'email_thread', ?)
            """, (db_source_id, thread_id, subject))
            
            cursor.execute("SELECT id FROM conversations WHERE source_id=? AND source_conversation_id=?", (db_source_id, thread_id))
            conv_id = cursor.fetchone()[0]
            
            # Insert Message
            cursor.execute("""
                INSERT OR IGNORE INTO messages 
                (conversation_id, source_id, source_message_id, message_type, sent_at, subject, is_read)
                VALUES (?, ?, ?, 'email', ?, ?, 1)
            """, (conv_id, db_source_id, msg_id, date_ts, subject))
            
            cursor.execute("SELECT id FROM messages WHERE source_id=? AND source_message_id=?", (db_source_id, msg_id))
            db_msg_row = cursor.fetchone()
            if db_msg_row:
                db_msg_id = db_msg_row[0]
                
                # Insert Body (simplified)
                body_text = ""
                body_html = ""
                if message.is_multipart():
                    for part in message.walk():
                        ctype = part.get_content_type()
                        if ctype == 'text/plain':
                            body_text += part.get_payload(decode=True).decode('utf-8', 'ignore')
                        elif ctype == 'text/html':
                            body_html += part.get_payload(decode=True).decode('utf-8', 'ignore')
                else:
                    body_text = message.get_payload(decode=True).decode('utf-8', 'ignore')
                
                cursor.execute("""
                    INSERT OR REPLACE INTO message_bodies (message_id, body_text, body_html)
                    VALUES (?, ?, ?)
                """, (db_msg_id, body_text, body_html))
                
                # Insert Raw
                cursor.execute("""
                    INSERT OR REPLACE INTO message_raw (message_id, raw_data, raw_format)
                    VALUES (?, ?, 'mime')
                """, (db_msg_id, message.as_bytes()))

                # Participants
                parts = get_participants(message)
                for p in parts:
                    # Insert Participant
                    cursor.execute("INSERT OR IGNORE INTO participants (email_address, display_name) VALUES (?, ?)", (p['email'], p['name']))
                    cursor.execute("SELECT id FROM participants WHERE email_address = ?", (p['email'],))
                    p_id = cursor.fetchone()[0]
                    
                    # Link to Message
                    recip_type = 'to'
                    if p['type'] == 'from': 
                        cursor.execute("UPDATE messages SET sender_id=? WHERE id=?", (p_id, db_msg_id))
                        continue # Sender is not a recipient in this schema logic usually, but let's see
                    elif p['type'] == 'cc': recip_type = 'cc'
                    
                    cursor.execute("""
                        INSERT OR IGNORE INTO message_recipients (message_id, participant_id, recipient_type, display_name)
                        VALUES (?, ?, ?, ?)
                    """, (db_msg_id, p_id, recip_type, p['name']))

        except Exception as e:
            print(f"Error processing message {i}: {e}")
            continue

        if i % 100 == 0:
            conn.commit()
            print(f"Processed {i} messages...")

    conn.commit()
    conn.close()
    print("Import complete.")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python3 import_mbox.py <mbox_file> <db_file>")
        sys.exit(1)
    import_mbox(sys.argv[1], sys.argv[2])
