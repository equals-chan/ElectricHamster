package net.lingala.zip4j;

import java.sql.*;

public class sqliteConnecter {
    private Statement state;
    private Connection conn;

    private String insert = "insert into t_pwd values(?,?,?)";
    sqliteConnecter() throws SQLException {
        String db = "db.sqlite3";
        this.conn = DriverManager.getConnection("jdbc:sqlite:" + db);
        this.state = conn.createStatement();
        init();
    }

    private void init() throws SQLException {
        this.state.execute("CREATE TABLE IF NOT EXISTS t_pwd" +
                "(id integer primary key,FolderName varchar(255),PassWord varchar(255))");

        state.close();
    }

    public void insert(int id , String Foldername , String PassWord) throws SQLException {
        PreparedStatement ps = conn.prepareStatement(insert);
        ps.setInt(1,id);
        ps.setString(2,Foldername);
        ps.setString(3,PassWord);
        ps.execute();
        ps.close();
    }

    public void close() throws SQLException {
        conn.close();
    }

}
