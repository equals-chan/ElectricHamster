package net.lingala.zip4j;

import java.io.File;
import java.sql.*;

public class sqliteConnecter {
    private Statement state;
    private Connection conn;

    //数据库主要操作

    //插入条目
    private String insert = "insert into t_pwd values(?,?,?,?)";

    //获取目前可用文件名
    private String getfilesLastNO = "select filesLastNO from t_zipNO where id = 1";

    //修改可用文件名
    private String addfilesLastNO = "update t_zipNO set filesLastNO=? where id=1";

    //查询是否已经压缩过了
    private String isZippedSQL = "select id from t_pwd where FolderName=?";

    //导出所有条目
    private String selectAll = "select * from t_pwd";

    sqliteConnecter(String dbPath) throws SQLException {
        String db = dbPath;

        File file = new File(db);
        boolean isex = file.exists();

        this.conn = DriverManager.getConnection("jdbc:sqlite:" + db);
        this.state = conn.createStatement();
        //如果文件不存在，就初始化 上面连接时会自动生成文件
        if (!isex){
            System.out.println("\u001b[1;42m 未检测到数据库，开始创建\u001b[0m");
            init();
        }

    }

    //若数据库不存在 则初始化
    private void init() throws SQLException {

        //创建密码表 id , 文件夹名 ， 密码 ，时间
        this.state.execute("CREATE TABLE IF NOT EXISTS t_pwd" +
                "(id integer primary key,FolderName varchar(255),PassWord varchar(255),createTime varchar(255))");

        //创建序号表 id ，目前序号 理论上序号就1行 id = 1
        this.state.execute("CREATE TABLE IF NOT EXISTS t_zipNO" +
                "(id integer primary key , filesLastNO integer )");

        //初始化序号表 仅当创建表时执行一次
        this.state.execute("insert into t_zipNO values(1,10000)");
        //关闭state
        state.close();
    }


    //插入数据 使用预编译的原因是文件夹名称复杂，可能导致sql注入问题
    public void insert(int id , String Foldername , String PassWord , String Time) throws SQLException {
        PreparedStatement ps = conn.prepareStatement(insert);
        ps.setInt(1,id);
        ps.setString(2,Foldername);
        ps.setString(3,PassWord);
        ps.setString(4,Time);
        ps.execute();
        ps.close();
    }

    // 获取当前可用的文件名
    public int getfilesLastNO() throws SQLException {
        PreparedStatement ps = conn.prepareStatement(getfilesLastNO);
        ResultSet resultSet = ps.executeQuery();
        int num = resultSet.getInt("filesLastNO");
        ps.close();
        return num;
    }

    // 用掉一个文件名，+1
    public void addfilesLastNO() throws SQLException{
        PreparedStatement ps = conn.prepareStatement(addfilesLastNO);
        ps.setInt(1,getfilesLastNO()+1);
        ps.executeUpdate();
        ps.close();
    }

    public boolean isZipped(String folderName) throws SQLException {
        PreparedStatement ps = conn.prepareStatement(isZippedSQL);
        ps.setString(1,folderName);
        ResultSet resultSet = ps.executeQuery();
        if (resultSet.next()){
            return true;
        }else return false;
    }

    public ResultSet selectAllData() throws SQLException {
        PreparedStatement ps = conn.prepareStatement(selectAll);
        ResultSet resultSet = ps.executeQuery();
        return resultSet;
    }

    //用完关闭数据库连接
    public void close() throws SQLException {
        conn.close();
    }

}
