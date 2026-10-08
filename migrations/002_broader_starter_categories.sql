-- Add broad starting points without changing or removing any user-owned taxonomy.
-- Every category remains editable, mergeable, deactivatable, and optional.
INSERT OR IGNORE INTO categories(name, normalized_name, description) VALUES
    ('Technology', 'technology', 'Technology products, trends, and industry'),
    ('Software Engineering', 'software engineering', 'Programming, architecture, tools, and engineering practice'),
    ('Science', 'science', 'Scientific discoveries, explanations, and disciplines'),
    ('Learning', 'learning', 'Courses, tutorials, study, and skill building'),
    ('Productivity', 'productivity', 'Workflows, habits, tools, and getting things done'),
    ('Design', 'design', 'Visual, product, interaction, and creative design'),
    ('Business', 'business', 'Companies, markets, management, and business strategy'),
    ('Finance', 'finance', 'Personal finance, investing, and economics'),
    ('Health & Fitness', 'health fitness', 'Health, exercise, nutrition, and wellbeing'),
    ('News & Politics', 'news politics', 'Current events, public policy, and politics'),
    ('Culture & Society', 'culture society', 'Culture, communities, history, and social topics'),
    ('Entertainment', 'entertainment', 'Film, television, music, games, and internet culture'),
    ('Personal', 'personal', 'Personal interests, reflections, and life reference');
